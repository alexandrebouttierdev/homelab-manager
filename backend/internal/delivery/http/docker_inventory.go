package httpdelivery

import (
	"net/http"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/usecase"
)

type dockerImageDTO struct {
	ID         string    `json:"id"`
	Tags       []string  `json:"tags"`
	CreatedAt  time.Time `json:"createdAt"`
	SizeBytes  int64     `json:"sizeBytes"`
	Containers int       `json:"containers"`
}

type dockerVolumeDTO struct {
	Name       string     `json:"name"`
	Driver     string     `json:"driver"`
	Scope      string     `json:"scope"`
	CreatedAt  *time.Time `json:"createdAt"`
	Containers int        `json:"containers"`
}

type dockerNetworkDTO struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Driver     string   `json:"driver"`
	Scope      string   `json:"scope"`
	Internal   bool     `json:"internal"`
	Predefined bool     `json:"predefined"`
	Subnets    []string `json:"subnets"`
	Containers int      `json:"containers"`
}

type dockerInventoryDTO struct {
	Images        []dockerImageDTO   `json:"images"`
	Volumes       []dockerVolumeDTO  `json:"volumes"`
	Networks      []dockerNetworkDTO `json:"networks"`
	Truncated     bool               `json:"truncated"`
	ObservedAt    *time.Time         `json:"observedAt"`
	Status        string             `json:"status"`
	LastSuccessAt *time.Time         `json:"lastSuccessAt"`
}

// dockerInventoryHandler serves the last images, volumes and networks read
// from Docker by the collection, used by a container or not.
func dockerInventoryHandler(monitoring *usecase.MonitoringUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Query()) > 0 {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "This route takes no query parameter.")
			return
		}
		inventory, status := monitoring.DockerInventory()
		writeJSON(w, http.StatusOK, dockerInventoryResponse(inventory, status))
	}
}

func dockerInventoryResponse(inventory *domain.DockerInventory, status domain.CollectorStatus) dockerInventoryDTO {
	response := dockerInventoryDTO{
		Images: []dockerImageDTO{}, Volumes: []dockerVolumeDTO{}, Networks: []dockerNetworkDTO{},
		Status: status.Status, LastSuccessAt: status.LastSuccessAt,
	}
	if inventory == nil {
		return response
	}
	observedAt := inventory.ObservedAt
	response.ObservedAt, response.Truncated = &observedAt, inventory.Truncated
	for _, image := range inventory.Images {
		tags := image.Tags
		if tags == nil {
			tags = []string{}
		}
		response.Images = append(response.Images, dockerImageDTO{
			ID: image.ID, Tags: tags, CreatedAt: image.CreatedAt, SizeBytes: image.SizeBytes, Containers: image.Containers,
		})
	}
	for _, volume := range inventory.Volumes {
		response.Volumes = append(response.Volumes, dockerVolumeDTO{
			Name: volume.Name, Driver: volume.Driver, Scope: volume.Scope, CreatedAt: volume.CreatedAt, Containers: volume.Containers,
		})
	}
	for _, network := range inventory.Networks {
		subnets := network.Subnets
		if subnets == nil {
			subnets = []string{}
		}
		response.Networks = append(response.Networks, dockerNetworkDTO{
			ID: network.ID, Name: network.Name, Driver: network.Driver, Scope: network.Scope,
			Internal: network.Internal, Predefined: network.Predefined, Subnets: subnets, Containers: network.Containers,
		})
	}
	return response
}
