// pr/types.go
package pr

type PR struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Author      string `json:"author"`
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
	URL         string `json:"url"`
	Mergeable   string `json:"mergeable"`
	CreatedAt   string `json:"createdAt"`
}
