package crawl

type StartTaskCommand struct {
	Subject        string
	OrganizationID string
	Keywords       []string
}

type OrganizationQuery struct {
	Subject        string
	OrganizationID string
}

type UpdateKeywordsCommand struct {
	Subject        string
	OrganizationID string
	Keywords       []string
}
