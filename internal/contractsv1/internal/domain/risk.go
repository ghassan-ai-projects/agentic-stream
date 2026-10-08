package domain

type RiskClass string

const (
	RiskR0 RiskClass = "R0"
	RiskR1 RiskClass = "R1"
	RiskR2 RiskClass = "R2"
	RiskR3 RiskClass = "R3"
	RiskR4 RiskClass = "R4"
)

type Route string

const (
	RouteAutomatic Route = "automatic"
	RouteApproval  Route = "approval"
	RouteDenied    Route = "denied"
)

type riskPolicyEntry struct {
	class RiskClass
	route Route
}

var riskPolicy = []riskPolicyEntry{
	{RiskR0, RouteAutomatic},
	{RiskR1, RouteAutomatic},
	{RiskR2, RouteApproval},
	{RiskR3, RouteDenied},
	{RiskR4, RouteDenied},
}

func RiskClasses() []RiskClass {
	classes := make([]RiskClass, 0, len(riskPolicy))
	for _, entry := range riskPolicy {
		classes = append(classes, entry.class)
	}
	return classes
}

func (r RiskClass) Valid() bool {
	return r.Rank() != 0
}

func (r RiskClass) Rank() int {
	for index, entry := range riskPolicy {
		if entry.class == r {
			return index + 1
		}
	}
	return 0
}

func (r RiskClass) AtMost(ceiling RiskClass) bool {
	return r.Rank() <= ceiling.Rank()
}

func (r RiskClass) baseRoute() Route {
	for _, entry := range riskPolicy {
		if entry.class == r {
			return entry.route
		}
	}
	return RouteDenied
}

func (r RiskClass) Consequential() bool {
	return r.Valid() && r.baseRoute() != RouteAutomatic
}

func (r RiskClass) Approvable() bool {
	return r.Valid() && r.baseRoute() != RouteDenied
}

func RouteFor(risk RiskClass, requiresApproval bool) Route {
	route := risk.baseRoute()
	if route == RouteAutomatic && requiresApproval {
		return RouteApproval
	}
	return route
}

func RiskPolicyDocument() map[string]any {
	document := make(map[string]any, len(riskPolicy))
	for _, entry := range riskPolicy {
		document[string(entry.class)] = string(entry.route)
	}
	return document
}

func IncompleteSourceHealthDocument() map[string]any {
	document := make(map[string]any)
	for _, class := range RiskClasses() {
		if class.Consequential() {
			document[string(class)] = string(RouteDenied)
		}
	}
	return document
}
