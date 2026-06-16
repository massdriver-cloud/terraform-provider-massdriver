package massdriver

// Shared test constants.
//
// The v1-era gqlmock-based newMockProvider helper is gone — under v2 each
// resource declares its own service interface (e.g. instanceAlarmsAPI) and
// tests inject hand-rolled fakes per ProviderClient field. There is no
// shared GraphQL transport mock to set up.
const testOrgID = "test-org"
