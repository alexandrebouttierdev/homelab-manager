package dockerengine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// fakeInventoryAPI answers the four listings of an inventory as Docker
// Engine API v1.41 does.
func fakeInventoryAPI(t *testing.T, failing string) *Client {
	t.Helper()
	answers := map[string]string{
		"/v1.41/containers/json": `[
			{"Id":"c1","ImageID":"sha256:nginx","Mounts":[{"Type":"volume","Name":"web-data"},{"Type":"volume","Name":"web-data"},{"Type":"bind","Source":"/srv"}],
			 "NetworkSettings":{"Networks":{"front":{"NetworkID":"net-front"},"bridge":{"NetworkID":"net-bridge"}}}},
			{"Id":"c2","ImageID":"sha256:nginx","Mounts":[],"NetworkSettings":{"Networks":{"front":{"NetworkID":"net-front"}}}}
		]`,
		"/v1.41/images/json": `[
			{"Id":"sha256:old","RepoTags":["<none>:<none>"],"Created":1700000000,"Size":1000,"Containers":-1},
			{"Id":"sha256:redis","RepoTags":["redis:7","redis:latest"],"Created":1700000100,"Size":2000,"Containers":-1},
			{"Id":"sha256:nginx","RepoTags":["nginx:stable"],"Created":1700000200,"Size":3000,"Containers":-1}
		]`,
		"/v1.41/volumes": `{"Volumes":[
			{"Name":"web-data","Driver":"local","Scope":"local","CreatedAt":"2026-11-20T10:00:00+01:00","Mountpoint":"/var/lib/docker/volumes/web-data/_data"},
			{"Name":"orphan","Driver":"local","Scope":"local","CreatedAt":"bad"}
		],"Warnings":null}`,
		"/v1.41/networks": `[
			{"Name":"front","Id":"net-front","Driver":"bridge","Scope":"local","IPAM":{"Config":[{"Subnet":"172.20.0.0/16","Gateway":"172.20.0.1"}]}},
			{"Name":"none","Id":"net-none","Driver":"null","Scope":"local","IPAM":{"Config":[]}},
			{"Name":"bridge","Id":"net-bridge","Driver":"bridge","Scope":"local","IPAM":{"Config":[{"Subnet":"172.17.0.0/16"}]}},
			{"Name":"old","Id":"net-old","Driver":"bridge","Scope":"local","Internal":true,"IPAM":{"Config":null}}
		]`,
	}
	handler := http.NewServeMux()
	for path, body := range answers {
		handler.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if path == failing {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if r.URL.RawQuery != "" && path != "/v1.41/containers/json" {
				t.Errorf("%s asked with %q", path, r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(body))
		})
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	transport := &http.Transport{}
	transport.Proxy = func(*http.Request) (*url.URL, error) { return url.Parse(server.URL) }
	return newClient(transport)
}

func TestInventoryCountsTheContainersOfEachItem(t *testing.T) {
	inventory, err := fakeInventoryAPI(t, "").Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	images := inventory.Images
	if len(images) != 3 || images[0].ID != "sha256:nginx" || images[0].Containers != 2 || images[0].SizeBytes != 3000 ||
		images[1].ID != "sha256:redis" || images[1].Containers != 0 || images[1].Tags[0] != "redis:7" ||
		images[2].ID != "sha256:old" || len(images[2].Tags) != 0 {
		t.Fatalf("images = %+v", images)
	}
	volumes := inventory.Volumes
	if len(volumes) != 2 || volumes[0].Name != "orphan" || volumes[0].Containers != 0 || volumes[0].CreatedAt != nil ||
		volumes[1].Name != "web-data" || volumes[1].Containers != 1 || volumes[1].CreatedAt.Location().String() != "UTC" {
		t.Fatalf("volumes = %+v", volumes)
	}
	networks := map[string]int{}
	for _, network := range inventory.Networks {
		networks[network.Name] = network.Containers
	}
	if networks["front"] != 2 || networks["bridge"] != 1 || networks["none"] != 0 || networks["old"] != 0 {
		t.Fatalf("networks = %+v", inventory.Networks)
	}
	for _, network := range inventory.Networks {
		if network.Predefined != (network.Name == "bridge" || network.Name == "none") {
			t.Fatalf("predefined %s = %t", network.Name, network.Predefined)
		}
		if network.Name == "front" && (len(network.Subnets) != 1 || network.Subnets[0] != "172.20.0.0/16") {
			t.Fatalf("subnets = %v", network.Subnets)
		}
	}
	if inventory.Truncated || inventory.ObservedAt.IsZero() {
		t.Fatalf("inventory = %+v", inventory)
	}
}

func TestInventoryFailsWithOneListing(t *testing.T) {
	for _, path := range []string{"/v1.41/containers/json", "/v1.41/images/json", "/v1.41/volumes", "/v1.41/networks"} {
		if _, err := fakeInventoryAPI(t, path).Inventory(context.Background()); err == nil {
			t.Fatalf("an inventory without %s succeeded", path)
		}
	}
}

func TestInventoryIsBounded(t *testing.T) {
	images := make([]imageSummary, 1001)
	for index := range images {
		images[index].ID = "sha256:" + string(rune('a'+index%26)) + string(rune('a'+index/26%26)) + string(rune('a'+index/676))
	}
	inventory := buildInventory(nil, images, nil, nil, time.Now())
	if len(inventory.Images) != 1000 || !inventory.Truncated {
		t.Fatalf("%d images, truncated %t", len(inventory.Images), inventory.Truncated)
	}
}
