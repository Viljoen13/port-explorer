package ports

// PortInfo holds information about a single network port entry.
type PortInfo struct {
	Protocol string `json:"protocol"`
	Port     uint16 `json:"port"`
	PID      int    `json:"pid"`
	Process  string `json:"process"`
	State    string `json:"state"`
	Address  string `json:"address"`
}

// List returns all port entries on the system.
// This function delegates to platform-specific implementations.
func List() ([]PortInfo, error) {
	return list()
}
