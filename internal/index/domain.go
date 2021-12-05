package index

type Domain struct {
	Packages map[string][]Version `json:"packages"`
}
