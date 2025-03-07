package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/graphql-go/graphql"
)

// buildDynamicSchema builds a GraphQL schema from the database structure
func buildDynamicSchema(db *sql.DB) (*graphql.Schema, error) {
	// Create a map to hold all the fields for the root query
	fields := graphql.Fields{}

	// Add a simple health check query
	fields["health"] = &graphql.Field{
		Type: graphql.String,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return "OK", nil
		},
	}

	// Get all tables in the database
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		return nil, fmt.Errorf("failed to query tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, fmt.Errorf("failed to scan table name: %w", err)
		}
		tables = append(tables, tableName)
	}

	// Process each table
	for _, tableName := range tables {
		// Skip internal tables
		if tableName == "sqlite_sequence" || tableName == "flow_metadata" {
			continue
		}

		log.Printf("Building schema for table: %s", tableName)

		// Get table schema
		schemaRows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
		if err != nil {
			log.Printf("Error getting schema for table %s: %v", tableName, err)
			continue
		}

		// Create fields for the type
		typeFields := graphql.Fields{}
		var columns []string
		var columnTypes []string

		for schemaRows.Next() {
			var cid int
			var name, typeName string
			var notNull, pk int
			var defaultValue interface{}

			if err := schemaRows.Scan(&cid, &name, &typeName, &notNull, &defaultValue, &pk); err != nil {
				log.Printf("Error scanning column info: %v", err)
				continue
			}

			columns = append(columns, name)
			columnTypes = append(columnTypes, typeName)

			// Map SQL type to GraphQL type
			var fieldType graphql.Type
			switch strings.ToUpper(typeName) {
			case "INTEGER", "INT", "SMALLINT", "MEDIUMINT", "BIGINT":
				fieldType = graphql.Int
			case "REAL", "FLOAT", "DOUBLE", "NUMERIC", "DECIMAL":
				fieldType = graphql.Float
			case "TEXT", "VARCHAR", "CHAR", "CLOB":
				fieldType = graphql.String
			case "BOOLEAN":
				fieldType = graphql.Boolean
			default:
				fieldType = graphql.String // Default to string for unknown types
			}

			// Make non-nullable if required
			if notNull == 1 && pk == 0 { // Primary keys can be auto-increment, so they might appear null initially
				fieldType = graphql.NewNonNull(fieldType)
			}

			typeFields[name] = &graphql.Field{
				Type: fieldType,
			}
		}
		schemaRows.Close()

		// Create the type
		typeName := pascalCase(singularize(tableName))
		objType := graphql.NewObject(graphql.ObjectConfig{
			Name:   typeName,
			Fields: typeFields,
		})

		// Create query for getting a single item by ID
		singleQueryName := camelCase(singularize(tableName))
		idField := findIdField(columns)
		if idField != "" {
			fields[singleQueryName] = &graphql.Field{
				Type: objType,
				Args: graphql.FieldConfigArgument{
					idField: &graphql.ArgumentConfig{
						Type: graphql.NewNonNull(graphql.String),
					},
				},
				Resolve: createSingleItemResolver(db, tableName, columns, idField),
			}
		}

		// Create query for getting a list of items
		listQueryName := camelCase(pluralize(tableName))
		fields[listQueryName] = &graphql.Field{
			Type: graphql.NewList(objType),
			Args: graphql.FieldConfigArgument{
				"first": &graphql.ArgumentConfig{
					Type:         graphql.Int,
					DefaultValue: 10,
					Description:  "Number of items to return",
				},
				"after": &graphql.ArgumentConfig{
					Type:        graphql.String,
					Description: "Cursor for pagination",
				},
			},
			Resolve: createListResolver(db, tableName, columns, idField),
		}
	}

	// Create the schema
	schemaConfig := graphql.SchemaConfig{
		Query: graphql.NewObject(graphql.ObjectConfig{
			Name:   "Query",
			Fields: fields,
		}),
	}

	schema, err := graphql.NewSchema(schemaConfig)
	if err != nil {
		return nil, err
	}

	return &schema, nil
}

// createSingleItemResolver creates a resolver for a single item query
func createSingleItemResolver(db *sql.DB, tableName string, columns []string, idField string) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		id, ok := p.Args[idField].(string)
		if !ok {
			return nil, fmt.Errorf("invalid ID argument")
		}

		log.Printf("Querying %s with %s=%s", tableName, idField, id)

		query := fmt.Sprintf("SELECT * FROM %s WHERE %s = ?", tableName, idField)
		row := db.QueryRow(query, id)

		// Create a map to hold the result
		result := make(map[string]interface{})
		scanArgs := make([]interface{}, len(columns))
		scanValues := make([]interface{}, len(columns))
		for i := range columns {
			scanValues[i] = new(interface{})
			scanArgs[i] = scanValues[i]
		}

		if err := row.Scan(scanArgs...); err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, fmt.Errorf("error scanning row: %w", err)
		}

		// Populate the result map
		for i, col := range columns {
			val := *(scanValues[i].(*interface{}))
			if val == nil {
				result[col] = nil
				continue
			}

			// Handle different types
			switch v := val.(type) {
			case []byte:
				// Try to convert to string
				result[col] = string(v)
			default:
				result[col] = v
			}
		}

		return result, nil
	}
}

// createListResolver creates a resolver for a list query
func createListResolver(db *sql.DB, tableName string, columns []string, idField string) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		limit, _ := p.Args["first"].(int)
		if limit <= 0 {
			limit = 10
		}

		after, _ := p.Args["after"].(string)

		log.Printf("Querying %s with limit=%d, after=%s", tableName, limit, after)

		query := fmt.Sprintf("SELECT * FROM %s", tableName)
		args := []interface{}{}

		if after != "" && idField != "" {
			query += fmt.Sprintf(" WHERE %s > ?", idField)
			args = append(args, after)
		}

		if idField != "" {
			query += fmt.Sprintf(" ORDER BY %s", idField)
		}

		query += " LIMIT ?"
		args = append(args, limit)

		rows, err := db.Query(query, args...)
		if err != nil {
			return nil, fmt.Errorf("error querying database: %w", err)
		}
		defer rows.Close()

		var results []interface{}
		for rows.Next() {
			// Create a map to hold the result
			result := make(map[string]interface{})
			scanArgs := make([]interface{}, len(columns))
			scanValues := make([]interface{}, len(columns))
			for i := range columns {
				scanValues[i] = new(interface{})
				scanArgs[i] = scanValues[i]
			}

			if err := rows.Scan(scanArgs...); err != nil {
				return nil, fmt.Errorf("error scanning row: %w", err)
			}

			// Populate the result map
			for i, col := range columns {
				val := *(scanValues[i].(*interface{}))
				if val == nil {
					result[col] = nil
					continue
				}

				// Handle different types
				switch v := val.(type) {
				case []byte:
					// Try to convert to string
					result[col] = string(v)
				default:
					result[col] = v
				}
			}

			results = append(results, result)
		}

		return results, nil
	}
}

// Helper functions

// findIdField finds the ID field in a list of columns
func findIdField(columns []string) string {
	// Common ID field names
	idFields := []string{"id", "account_id", "sequence", "hash"}

	for _, field := range idFields {
		for _, col := range columns {
			if strings.EqualFold(col, field) {
				return col
			}
		}
	}

	return ""
}

// singularize converts a plural word to singular
func singularize(s string) string {
	if strings.HasSuffix(s, "ies") {
		return s[:len(s)-3] + "y"
	}
	if strings.HasSuffix(s, "s") && !strings.HasSuffix(s, "ss") {
		return s[:len(s)-1]
	}
	return s
}

// pluralize converts a singular word to plural
func pluralize(s string) string {
	if strings.HasSuffix(s, "y") {
		return s[:len(s)-1] + "ies"
	}
	if !strings.HasSuffix(s, "s") {
		return s + "s"
	}
	return s
}

// pascalCase converts a string to PascalCase
func pascalCase(s string) string {
	words := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})

	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}

	return strings.Join(words, "")
}

// camelCase converts a string to camelCase
func camelCase(s string) string {
	pascal := pascalCase(s)
	if len(pascal) > 0 {
		return strings.ToLower(pascal[:1]) + pascal[1:]
	}
	return ""
}
