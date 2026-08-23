package main

type field struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type geoResult struct {
	IP     string  `json:"ip,omitempty"`
	Family string  `json:"family,omitempty"`
	Source string  `json:"source,omitempty"`
	Error  string  `json:"error,omitempty"`
	Raw    string  `json:"raw,omitempty"`
	Fields []field `json:"fields,omitempty"`
}

type result struct {
	LocalIPv4   string     `json:"local_ipv4,omitempty"`
	InterfaceV4 string     `json:"interface_v4,omitempty"`
	LocalIPv6   string     `json:"local_ipv6,omitempty"`
	InterfaceV6 string     `json:"interface_v6,omitempty"`
	PublicIPv4  *geoResult `json:"public_ipv4"`
	PublicIPv6  *geoResult `json:"public_ipv6"`
}
