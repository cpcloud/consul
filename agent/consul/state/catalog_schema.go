package state

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hashicorp/go-memdb"

	"github.com/hashicorp/consul/agent/structs"
)

// nodesTableSchema returns a new table schema used for storing node
// information.
func nodesTableSchema() *memdb.TableSchema {
	return &memdb.TableSchema{
		Name: "nodes",
		Indexes: map[string]*memdb.IndexSchema{
			"id": {
				Name:         "id",
				AllowMissing: false,
				Unique:       true,
				Indexer: &memdb.StringFieldIndex{
					Field:     "Node",
					Lowercase: true,
				},
			},
			"uuid": {
				Name:         "uuid",
				AllowMissing: true,
				Unique:       true,
				Indexer: &memdb.UUIDFieldIndex{
					Field: "ID",
				},
			},
			"meta": {
				Name:         "meta",
				AllowMissing: true,
				Unique:       false,
				Indexer: &memdb.StringMapFieldIndex{
					Field:     "Meta",
					Lowercase: false,
				},
			},
		},
	}
}

// servicesTableSchema returns a new table schema used to store information
// about services.
func servicesTableSchema() *memdb.TableSchema {
	return &memdb.TableSchema{
		Name: "services",
		Indexes: map[string]*memdb.IndexSchema{
			"id": {
				Name:         "id",
				AllowMissing: false,
				Unique:       true,
				Indexer: withNamespaceIndex(&memdb.CompoundIndex{
					Indexes: []memdb.Indexer{
						&memdb.StringFieldIndex{
							Field:     "Node",
							Lowercase: true,
						},
						&memdb.StringFieldIndex{
							Field:     "ServiceID",
							Lowercase: true,
						},
					},
				}),
			},
			"node": {
				Name:         "node",
				AllowMissing: false,
				Unique:       false,
				Indexer: &memdb.StringFieldIndex{
					Field:     "Node",
					Lowercase: true,
				},
			},
			"service": {
				Name:         "service",
				AllowMissing: true,
				Unique:       false,
				Indexer: withNamespaceIndex(&memdb.StringFieldIndex{
					Field:     "ServiceName",
					Lowercase: true,
				}),
			},
			"connect": {
				Name:         "connect",
				AllowMissing: true,
				Unique:       false,
				Indexer:      withNamespaceIndex(&IndexConnectService{}),
			},
			"kind": {
				Name:         "kind",
				AllowMissing: false,
				Unique:       false,
				Indexer:      withNamespaceMultiIndex(&IndexServiceKind{}),
			},
		},
	}
}

// checksTableSchema returns a new table schema used for storing and indexing
// health check information. Health checks have a number of different attributes
// we want to filter by, so this table is a bit more complex.
func checksTableSchema() *memdb.TableSchema {
	return &memdb.TableSchema{
		Name: "checks",
		Indexes: map[string]*memdb.IndexSchema{
			"id": {
				Name:         "id",
				AllowMissing: false,
				Unique:       true,
				Indexer: withNamespaceIndex(&memdb.CompoundIndex{
					Indexes: []memdb.Indexer{
						&memdb.StringFieldIndex{
							Field:     "Node",
							Lowercase: true,
						},
						&memdb.StringFieldIndex{
							Field:     "CheckID",
							Lowercase: true,
						},
					},
				}),
			},
			"status": {
				Name:         "status",
				AllowMissing: false,
				Unique:       false,
				Indexer: withNamespaceMultiIndex(&memdb.StringFieldIndex{
					Field:     "Status",
					Lowercase: false,
				}),
			},
			"service": {
				Name:         "service",
				AllowMissing: true,
				Unique:       false,
				Indexer: withNamespaceIndex(&memdb.StringFieldIndex{
					Field:     "ServiceName",
					Lowercase: true,
				}),
			},
			"node": {
				Name:         "node",
				AllowMissing: true,
				Unique:       false,
				Indexer: &memdb.StringFieldIndex{
					Field:     "Node",
					Lowercase: true,
				},
			},
			"node_service_check": {
				Name:         "node_service_check",
				AllowMissing: true,
				Unique:       false,
				Indexer: withNamespaceIndex(&memdb.CompoundIndex{
					Indexes: []memdb.Indexer{
						&memdb.StringFieldIndex{
							Field:     "Node",
							Lowercase: true,
						},
						&memdb.FieldSetIndex{
							Field: "ServiceID",
						},
					},
				}),
			},
			"node_service": {
				Name:         "node_service",
				AllowMissing: true,
				Unique:       false,
				Indexer: withNamespaceIndex(&memdb.CompoundIndex{
					Indexes: []memdb.Indexer{
						&memdb.StringFieldIndex{
							Field:     "Node",
							Lowercase: true,
						},
						&memdb.StringFieldIndex{
							Field:     "ServiceID",
							Lowercase: true,
						},
					},
				}),
			},
		},
	}
}

//  gatewayServicesTableNameSchema returns a new table schema used to store information
// about services associated with terminating gateways.
func gatewayServicesTableNameSchema() *memdb.TableSchema {
	return &memdb.TableSchema{
		Name: gatewayServicesTableName,
		Indexes: map[string]*memdb.IndexSchema{
			"id": {
				Name:         "id",
				AllowMissing: false,
				Unique:       true,
				Indexer: &memdb.CompoundIndex{
					Indexes: []memdb.Indexer{
						&ServiceNameIndex{
							Field: "Gateway",
						},
						&ServiceNameIndex{
							Field: "Service",
						},
						&memdb.IntFieldIndex{
							Field: "Port",
						},
					},
				},
			},
			"gateway": {
				Name:         "gateway",
				AllowMissing: false,
				Unique:       false,
				Indexer: &ServiceNameIndex{
					Field: "Gateway",
				},
			},
			"service": {
				Name:         "service",
				AllowMissing: true,
				Unique:       false,
				Indexer: &ServiceNameIndex{
					Field: "Service",
				},
			},
		},
	}
}

// topologyTableNameSchema returns a new table schema used to store information
// relating upstream and downstream services
func topologyTableNameSchema() *memdb.TableSchema {
	return &memdb.TableSchema{
		Name: topologyTableName,
		Indexes: map[string]*memdb.IndexSchema{
			"id": {
				Name:         "id",
				AllowMissing: false,
				Unique:       true,
				Indexer: &memdb.CompoundIndex{
					Indexes: []memdb.Indexer{
						&ServiceNameIndex{
							Field: "Upstream",
						},
						&ServiceNameIndex{
							Field: "Downstream",
						},
					},
				},
			},
			"upstream": {
				Name:         "upstream",
				AllowMissing: true,
				Unique:       false,
				Indexer: &ServiceNameIndex{
					Field: "Upstream",
				},
			},
			"downstream": {
				Name:         "downstream",
				AllowMissing: false,
				Unique:       false,
				Indexer: &ServiceNameIndex{
					Field: "Downstream",
				},
			},
		},
	}
}

type ServiceNameIndex struct {
	Field string
}

func (index *ServiceNameIndex) FromObject(obj interface{}) (bool, []byte, error) {
	v := reflect.ValueOf(obj)
	v = reflect.Indirect(v) // Dereference the pointer if any

	fv := v.FieldByName(index.Field)
	isPtr := fv.Kind() == reflect.Ptr
	fv = reflect.Indirect(fv)
	if !isPtr && !fv.IsValid() || !fv.CanInterface() {
		return false, nil,
			fmt.Errorf("field '%s' for %#v is invalid %v ", index.Field, obj, isPtr)
	}

	name, ok := fv.Interface().(structs.ServiceName)
	if !ok {
		return false, nil, fmt.Errorf("Field 'ServiceName' is not of type structs.ServiceName")
	}

	// Enforce lowercase and add null character as terminator
	id := strings.ToLower(name.String()) + "\x00"

	return true, []byte(id), nil
}

func (index *ServiceNameIndex) FromArgs(args ...interface{}) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("must provide only a single argument")
	}
	name, ok := args[0].(structs.ServiceName)
	if !ok {
		return nil, fmt.Errorf("argument must be of type structs.ServiceName: %#v", args[0])
	}

	// Enforce lowercase and add null character as terminator
	id := strings.ToLower(name.String()) + "\x00"

	return []byte(strings.ToLower(id)), nil
}

func (index *ServiceNameIndex) PrefixFromArgs(args ...interface{}) ([]byte, error) {
	val, err := index.FromArgs(args...)
	if err != nil {
		return nil, err
	}

	// Strip the null terminator, the rest is a prefix
	n := len(val)
	if n > 0 {
		return val[:n-1], nil
	}
	return val, nil
}

func init() {
	registerSchema(nodesTableSchema)
	registerSchema(servicesTableSchema)
	registerSchema(checksTableSchema)
	registerSchema(gatewayServicesTableNameSchema)
	registerSchema(topologyTableNameSchema)
}
