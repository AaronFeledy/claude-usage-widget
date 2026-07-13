package cursor

type cursorUsageSummary struct {
	BillingCycleEnd *string                `json:"billingCycleEnd"`
	MembershipType  *string                `json:"membershipType"`
	IndividualUsage *cursorIndividualUsage `json:"individualUsage"`
}

type cursorIndividualUsage struct {
	Plan     *cursorPlanUsage     `json:"plan"`
	OnDemand *cursorOnDemandUsage `json:"onDemand"`
}

type cursorPlanUsage struct {
	Used             *int     `json:"used"`
	Limit            *int     `json:"limit"`
	TotalPercentUsed *float64 `json:"totalPercentUsed"`
}

type cursorOnDemandUsage struct {
	Used  *int `json:"used"`
	Limit *int `json:"limit"`
}

type cursorUserInfo struct {
	Sub string `json:"sub"`
}

type cursorUsageResponse struct {
	GPT4 *cursorModelUsage `json:"gpt-4"`
}

type cursorModelUsage struct {
	NumRequests      *int `json:"numRequests"`
	NumRequestsTotal *int `json:"numRequestsTotal"`
	MaxRequestUsage  *int `json:"maxRequestUsage"`
}
