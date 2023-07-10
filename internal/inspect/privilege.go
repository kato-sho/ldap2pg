package inspect

import (
	"context"
	"fmt"

	"github.com/dalibo/ldap2pg/internal/postgres"
	"github.com/dalibo/ldap2pg/internal/privilege"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/exp/slog"
)

func (instance *Instance) InspectStage2(ctx context.Context, pc Config) (err error) {
	err = instance.InspectGrants(ctx, pc.ManagedPrivileges)
	return
}

func (instance *Instance) InspectGrants(ctx context.Context, managedPrivileges map[string][]string) error {
	slog.Info("Inspecting privileges.")
	for _, p := range privilege.Map {
		managedTypes := managedPrivileges[p.Object]
		if 0 == len(managedTypes) {
			continue
		}
		var databases []string
		if "instance" == p.Scope {
			databases = []string{instance.DefaultDatabase}
		} else {
			databases = maps.Keys(instance.Databases)
		}

		for _, database := range databases {
			slog.Debug("Inspecting grants.", "scope", p.Scope, "database", database, "object", p.Object, "types", managedTypes)
			pgconn, err := postgres.DBPool.Get(ctx, database)
			if err != nil {
				return err
			}

			slog.Debug("Executing SQL query:\n"+p.Inspect, "arg", managedTypes)
			rows, err := pgconn.Query(ctx, p.Inspect, managedTypes)
			if err != nil {
				return fmt.Errorf("bad query: %w", err)
			}
			for rows.Next() {
				grant, err := privilege.RowTo(rows)
				if err != nil {
					return fmt.Errorf("bad row: %w", err)
				}
				grant.Target = p.Object

				database, known := instance.Databases[grant.Database]
				if !known {
					continue
				}
				_, known = database.Schemas[grant.Schema]
				if !known {
					continue
				}

				pattern := instance.RolesBlacklist.MatchString(grant.Grantee)
				if pattern != "" {
					slog.Debug(
						"Ignoring grant to blacklisted role.",
						"grant", grant, "pattern", pattern)
					continue
				}

				grant.Normalize()

				slog.Debug("Found grant in Postgres instance.", "grant", grant)
				instance.Grants = append(instance.Grants, grant)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("%s: %w", p, err)
			}

		}
	}
	return nil
}

func (instance *Instance) InspectSchemas(ctx context.Context, managedQuery Querier[postgres.Schema]) error {
	sq := &SQLQuery[postgres.Schema]{SQL: schemasQuery, RowTo: postgres.RowToSchema}
	for i, database := range instance.Databases {
		var managedSchemas []string
		slog.Debug("Inspecting managed schemas.", "database", database.Name)
		conn, err := postgres.DBPool.Get(ctx, database.Name)
		if err != nil {
			return err
		}
		for managedQuery.Query(ctx, conn); managedQuery.Next(); {
			s := managedQuery.Row()
			managedSchemas = append(managedSchemas, s.Name)
		}
		err = managedQuery.Err()
		if err != nil {
			return fmt.Errorf("schemas: %w", err)
		}

		for sq.Query(ctx, conn); sq.Next(); {
			s := sq.Row()
			if !slices.Contains(managedSchemas, s.Name) {
				continue
			}
			database.Schemas[s.Name] = s
			slog.Debug("Found schema.", "db", database.Name, "schema", s.Name, "owner", s.Owner)
		}
		err = sq.Err()
		if err != nil {
			return fmt.Errorf("schemas: %w", err)
		}

		instance.Databases[i] = database
	}

	return nil
}
