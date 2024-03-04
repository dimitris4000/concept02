// jsonrequests.go holds all the JSON schemas related to http requests
// concept02 service is expected to handle

package service

type JsonResourceSpecifier struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}
