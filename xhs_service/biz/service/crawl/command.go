package crawl

type StartTaskCommand struct {
	Subject        string
	Scopes         []string
	OrganizationID string
	Keywords       []string
}

type OrganizationQuery struct {
	Subject        string
	Scopes         []string
	OrganizationID string
}

type UpdateKeywordsCommand struct {
	Subject        string
	Scopes         []string
	OrganizationID string
	Keywords       []string
}
