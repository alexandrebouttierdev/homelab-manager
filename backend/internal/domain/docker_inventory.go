package domain

import "time"

// Bounds of one Docker inventory: a larger list is cut and marked
// truncated.
const (
	MaxInventoryImages   = 1000
	MaxInventoryVolumes  = 1000
	MaxInventoryNetworks = 200
)

// PredefinedDockerNetworks are created by Docker itself and never pruned:
// they are not "unused" even without a container.
var PredefinedDockerNetworks = map[string]bool{"bridge": true, "host": true, "none": true}

// DockerImage is an image of the Docker host. Containers counts the
// containers, running or not, created from it.
type DockerImage struct {
	ID         string
	Tags       []string
	CreatedAt  time.Time
	SizeBytes  int64
	Containers int
}

// DockerVolume is a named volume. Containers counts the containers,
// running or not, that mount it.
type DockerVolume struct {
	Name       string
	Driver     string
	Scope      string
	CreatedAt  *time.Time
	Containers int
}

// DockerNetwork is a network of the Docker host. Containers counts the
// containers, running or not, attached to it.
type DockerNetwork struct {
	ID         string
	Name       string
	Driver     string
	Scope      string
	Internal   bool
	Predefined bool
	Subnets    []string
	Containers int
}

// DockerInventory is the images, volumes and networks of the Docker host
// read in one cycle, with or without containers.
type DockerInventory struct {
	Images     []DockerImage
	Volumes    []DockerVolume
	Networks   []DockerNetwork
	Truncated  bool
	ObservedAt time.Time
}
