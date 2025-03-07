package schemaapi

// SchemaRegistration represents a schema registration request
type SchemaRegistration struct {
	PluginName string `json:"plugin_name"`
	Schema     string `json:"schema"`
	Queries    string `json:"queries"`
}

// SchemaProvider is an interface for plugins that provide GraphQL schema components
type SchemaProvider interface {
	// GetSchemaDefinition returns GraphQL type definitions for this plugin
	GetSchemaDefinition() string

	// GetQueryDefinitions returns GraphQL query definitions for this plugin
	GetQueryDefinitions() string
}
