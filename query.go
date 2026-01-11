package vessel

// ServiceQuery defines criteria for querying services.
type ServiceQuery struct {
	// Lifecycle filters by service lifecycle (singleton, transient, scoped).
	// Empty string matches all lifecycles.
	Lifecycle string

	// Group filters by service group.
	// Empty string matches all groups.
	Group string

	// Metadata filters by service metadata key-value pairs.
	// All specified metadata must match for a service to be included.
	Metadata map[string]string

	// Started filters by whether the service has been started.
	// nil matches all services (started and not started).
	Started *bool
}

// Query returns detailed information about services matching the query criteria.
// Returns a slice of ServiceInfo for all matching services.
//
// Example:
//
//	// Find all singleton services in the "api" group
//	started := true
//	results := vessel.Query(c, vessel.ServiceQuery{
//	    Lifecycle: "singleton",
//	    Group: "api",
//	    Started: &started,
//	})
func Query(c Vessel, query ServiceQuery) []ServiceInfo {
	allServices := c.Services()
	var results []ServiceInfo

	for _, name := range allServices {
		info := c.Inspect(name)

		// Filter by lifecycle
		if query.Lifecycle != "" && info.Lifecycle != query.Lifecycle {
			continue
		}

		// Filter by group
		if query.Group != "" {
			hasGroup := false
			for _, group := range extractGroups(info) {
				if group == query.Group {
					hasGroup = true
					break
				}
			}
			if !hasGroup {
				continue
			}
		}

		// Filter by metadata
		if len(query.Metadata) > 0 {
			allMatch := true
			for key, value := range query.Metadata {
				if info.Metadata[key] != value {
					allMatch = false
					break
				}
			}
			if !allMatch {
				continue
			}
		}

		// Filter by started status
		if query.Started != nil && info.Started != *query.Started {
			continue
		}

		results = append(results, info)
	}

	return results
}

// QueryNames returns the names of services matching the query criteria.
// This is more efficient than Query when you only need service names.
//
// Example:
//
//	// Find all service names in the "db" group
//	names := vessel.QueryNames(c, vessel.ServiceQuery{
//	    Group: "db",
//	})
func QueryNames(c Vessel, query ServiceQuery) []string {
	results := Query(c, query)
	names := make([]string, len(results))
	for i, info := range results {
		names[i] = info.Name
	}
	return names
}

// FindByGroup returns all services in a specific group.
func FindByGroup(c Vessel, group string) []ServiceInfo {
	return Query(c, ServiceQuery{Group: group})
}

// FindByLifecycle returns all services with a specific lifecycle.
func FindByLifecycle(c Vessel, lifecycle string) []ServiceInfo {
	return Query(c, ServiceQuery{Lifecycle: lifecycle})
}

// FindStarted returns all services that have been started.
func FindStarted(c Vessel) []ServiceInfo {
	started := true
	return Query(c, ServiceQuery{Started: &started})
}

// FindNotStarted returns all services that have not been started.
func FindNotStarted(c Vessel) []ServiceInfo {
	started := false
	return Query(c, ServiceQuery{Started: &started})
}

// extractGroups extracts group names from ServiceInfo.
// Groups might be stored in different places depending on how they were registered.
func extractGroups(info ServiceInfo) []string {
	// Check if groups are stored in __groups metadata (comma-separated)
	if groupStr, ok := info.Metadata["__groups"]; ok && groupStr != "" {
		return splitStrings(groupStr, ",")
	}

	return nil
}

// splitStrings is a helper to split strings.
func splitStrings(s, sep string) []string {
	if s == "" {
		return nil
	}
	result := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep[0] {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}
