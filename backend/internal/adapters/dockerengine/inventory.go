package dockerengine

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"homelab-manager/backend/internal/domain"
)

// Inventory reads the images, volumes and networks of the Docker host and
// counts the containers, running or not, that use each one.
func (c *Client) Inventory(ctx context.Context) (domain.DockerInventory, error) {
	var containers []usageSummary
	if err := c.get(ctx, "/containers/json?all=1", &containers); err != nil {
		return domain.DockerInventory{}, err
	}
	var images []imageSummary
	if err := c.get(ctx, "/images/json", &images); err != nil {
		return domain.DockerInventory{}, fmt.Errorf("list images: %w", err)
	}
	var volumes volumeList
	if err := c.get(ctx, "/volumes", &volumes); err != nil {
		return domain.DockerInventory{}, fmt.Errorf("list volumes: %w", err)
	}
	var networks []networkSummary
	if err := c.get(ctx, "/networks", &networks); err != nil {
		return domain.DockerInventory{}, fmt.Errorf("list networks: %w", err)
	}
	return buildInventory(containers, images, volumes.Volumes, networks, time.Now().UTC()), nil
}

func buildInventory(containers []usageSummary, images []imageSummary, volumes []volumeSummary, networks []networkSummary, now time.Time) domain.DockerInventory {
	imageUse, volumeUse, networkUse := map[string]int{}, map[string]int{}, map[string]int{}
	for _, container := range containers {
		imageUse[container.ImageID]++
		volumesSeen := map[string]bool{}
		for _, mount := range container.Mounts {
			if mount.Type == "volume" && mount.Name != "" && !volumesSeen[mount.Name] {
				volumesSeen[mount.Name] = true
				volumeUse[mount.Name]++
			}
		}
		for name, endpoint := range container.NetworkSettings.Networks {
			var attachment struct {
				NetworkID string `json:"NetworkID"`
			}
			if json.Unmarshal(endpoint, &attachment) == nil && attachment.NetworkID != "" {
				networkUse["id:"+attachment.NetworkID]++
			} else {
				networkUse["name:"+name]++
			}
		}
	}

	inventory := domain.DockerInventory{ObservedAt: now}
	for _, image := range images {
		tags := make([]string, 0, len(image.RepoTags))
		for _, tag := range image.RepoTags {
			if tag != "" && tag != "<none>:<none>" {
				tags = append(tags, tag)
			}
		}
		sort.Strings(tags)
		inventory.Images = append(inventory.Images, domain.DockerImage{
			ID: image.ID, Tags: tags, CreatedAt: time.Unix(image.Created, 0).UTC(),
			SizeBytes: max(image.Size, 0), Containers: imageUse[image.ID],
		})
	}
	sort.Slice(inventory.Images, func(i, j int) bool {
		left, right := inventory.Images[i], inventory.Images[j]
		if (len(left.Tags) == 0) != (len(right.Tags) == 0) {
			return len(left.Tags) > 0
		}
		if len(left.Tags) > 0 && left.Tags[0] != right.Tags[0] {
			return left.Tags[0] < right.Tags[0]
		}
		return left.ID < right.ID
	})
	for _, volume := range volumes {
		item := domain.DockerVolume{
			Name: volume.Name, Driver: volume.Driver, Scope: volume.Scope, Containers: volumeUse[volume.Name],
		}
		if created, err := time.Parse(time.RFC3339Nano, volume.CreatedAt); err == nil {
			created = created.UTC()
			item.CreatedAt = &created
		}
		inventory.Volumes = append(inventory.Volumes, item)
	}
	sort.Slice(inventory.Volumes, func(i, j int) bool { return inventory.Volumes[i].Name < inventory.Volumes[j].Name })
	for _, network := range networks {
		subnets := make([]string, 0, len(network.IPAM.Config))
		for _, config := range network.IPAM.Config {
			if config.Subnet != "" {
				subnets = append(subnets, config.Subnet)
			}
		}
		inventory.Networks = append(inventory.Networks, domain.DockerNetwork{
			ID: network.ID, Name: network.Name, Driver: network.Driver, Scope: network.Scope,
			Internal: network.Internal, Predefined: domain.PredefinedDockerNetworks[network.Name],
			Subnets:    subnets,
			Containers: networkUse["id:"+network.ID] + networkUse["name:"+network.Name],
		})
	}
	sort.Slice(inventory.Networks, func(i, j int) bool { return inventory.Networks[i].Name < inventory.Networks[j].Name })

	if len(inventory.Images) > domain.MaxInventoryImages {
		inventory.Images, inventory.Truncated = inventory.Images[:domain.MaxInventoryImages], true
	}
	if len(inventory.Volumes) > domain.MaxInventoryVolumes {
		inventory.Volumes, inventory.Truncated = inventory.Volumes[:domain.MaxInventoryVolumes], true
	}
	if len(inventory.Networks) > domain.MaxInventoryNetworks {
		inventory.Networks, inventory.Truncated = inventory.Networks[:domain.MaxInventoryNetworks], true
	}
	return inventory
}

// usageSummary is the part of a container listing that tells which
// image, volumes and networks it uses.
type usageSummary struct {
	ImageID string `json:"ImageID"`
	Mounts  []struct {
		Type string `json:"Type"`
		Name string `json:"Name"`
	} `json:"Mounts"`
	NetworkSettings struct {
		Networks map[string]json.RawMessage `json:"Networks"`
	} `json:"NetworkSettings"`
}

type imageSummary struct {
	ID       string   `json:"Id"`
	RepoTags []string `json:"RepoTags"`
	Created  int64    `json:"Created"`
	Size     int64    `json:"Size"`
}

type volumeList struct {
	Volumes []volumeSummary `json:"Volumes"`
}

type volumeSummary struct {
	Name      string `json:"Name"`
	Driver    string `json:"Driver"`
	Scope     string `json:"Scope"`
	CreatedAt string `json:"CreatedAt"`
}

type networkSummary struct {
	ID       string `json:"Id"`
	Name     string `json:"Name"`
	Driver   string `json:"Driver"`
	Scope    string `json:"Scope"`
	Internal bool   `json:"Internal"`
	IPAM     struct {
		Config []struct {
			Subnet string `json:"Subnet"`
		} `json:"Config"`
	} `json:"IPAM"`
}
