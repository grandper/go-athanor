package http

// VersionInfo describes the version, commit, and build date of the running service.
type VersionInfo struct {
	// Version is the version of the running service, such as "1.2.3".
	Version string `json:"version"`

	// Commit is the identifier of the commit the service was built from.
	Commit string `json:"commit"`

	// BuildDate is the date the service was built on.
	BuildDate string `json:"build_date"`
}
