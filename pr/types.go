// pr/types.go
package pr

import "encoding/json"

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

// gh pr list returns author as an object {"login":"...","is_bot":...},
// not a plain string. Unmarshal into Author as the login string.
func (p *PR) UnmarshalJSON(data []byte) error {
	// Alias to avoid recursion
	type raw struct {
		Number      int             `json:"number"`
		Title       string          `json:"title"`
		Author      json.RawMessage `json:"author"`
		HeadRefName string          `json:"headRefName"`
		BaseRefName string          `json:"baseRefName"`
		URL         string          `json:"url"`
		Mergeable   string          `json:"mergeable"`
		CreatedAt   string          `json:"createdAt"`
	}
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	p.Number = r.Number
	p.Title = r.Title
	p.HeadRefName = r.HeadRefName
	p.BaseRefName = r.BaseRefName
	p.URL = r.URL
	p.Mergeable = r.Mergeable
	p.CreatedAt = r.CreatedAt

	// author can be a string (legacy) or an object {"login":"..."}
	switch r.Author[0] {
	case '"':
		var s string
		if err := json.Unmarshal(r.Author, &s); err != nil {
			return err
		}
		p.Author = s
	case '{':
		var obj struct {
			Login string `json:"login"`
		}
		if err := json.Unmarshal(r.Author, &obj); err != nil {
			return err
		}
		p.Author = obj.Login
	}
	return nil
}
