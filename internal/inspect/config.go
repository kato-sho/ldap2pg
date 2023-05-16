package inspect

type Config struct {
	FallbackOwner       string          `mapstructure:"fallback_owner"`
	DatabasesQuery      Querier[string] `mapstructure:"databases_query"`
	ManagedRolesQuery   RowsOrSQL       `mapstructure:"managed_roles_query"`
	RolesBlacklistQuery RowsOrSQL       `mapstructure:"roles_blacklist_query"`
}
