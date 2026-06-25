package domain

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	mercariSearchHost = "jp.mercari.com"
	itemPathPattern   = regexp.MustCompile(`^/(?:en/)?item/([a-zA-Z0-9]+)$`)
	searchPathPattern = regexp.MustCompile(`^/(?:en/)?search`)
)

var statusMap = map[string]string{
	"on_sale":  "STATUS_ON_SALE",
	"sold_out": "STATUS_SOLD_OUT",
}

var sortMap = map[string]string{
	"created_time": "SORT_CREATED_TIME",
	"price":        "SORT_PRICE",
	"score":        "SORT_SCORE",
	"num_likes":    "SORT_NUM_LIKES",
}

var orderMap = map[string]string{
	"desc": "ORDER_DESC",
	"asc":  "ORDER_ASC",
}

var reverseSortMap = map[string]string{
	"SORT_CREATED_TIME": "created_time",
	"SORT_PRICE":        "price",
	"SORT_SCORE":        "score",
	"SORT_NUM_LIKES":    "num_likes",
}

var reverseOrderMap = map[string]string{
	"ORDER_DESC": "desc",
	"ORDER_ASC":  "asc",
}

var reverseStatusMap = map[string]string{
	"STATUS_ON_SALE":  "on_sale",
	"STATUS_SOLD_OUT": "sold_out",
}

func IsMercariURL(text string) bool {
	return strings.Contains(text, mercariSearchHost)
}

func ParseInput(input string) SearchParams {
	input = strings.TrimSpace(input)

	u, err := url.Parse(input)
	if err != nil || u.Host == "" {
		return KeywordSearch(input)
	}

	if !strings.Contains(u.Host, mercariSearchHost) {
		return KeywordSearch(input)
	}

	if matches := itemPathPattern.FindStringSubmatch(u.Path); len(matches) == 2 {
		return SearchParams{ItemID: matches[1]}
	}

	if !searchPathPattern.MatchString(u.Path) {
		return KeywordSearch(input)
	}

	return parseSearchURL(u.Query())
}

func parseSearchURL(q url.Values) SearchParams {
	p := SearchParams{
		Keyword: q.Get("keyword"),
		Sort:    "SORT_CREATED_TIME",
		Order:   "ORDER_DESC",
	}

	if s, ok := sortMap[q.Get("sort")]; ok {
		p.Sort = s
	}

	if o, ok := orderMap[q.Get("order")]; ok {
		p.Order = o
	}

	if status := q.Get("status"); status != "" {
		for _, s := range strings.Split(status, ",") {
			if mapped, ok := statusMap[s]; ok {
				p.Status = append(p.Status, mapped)
			}
		}
	}
	if len(p.Status) == 0 {
		p.Status = []string{"STATUS_ON_SALE"}
	}

	if v, err := strconv.Atoi(q.Get("price_min")); err == nil {
		p.PriceMin = v
	}
	if v, err := strconv.Atoi(q.Get("price_max")); err == nil {
		p.PriceMax = v
	}

	p.ExcludeKeyword = q.Get("exclude_keyword")
	p.CategoryID = splitParam(q.Get("category_id"))
	p.BrandID = splitParam(q.Get("brand_id"))
	p.ItemConditionID = splitIntParam(q.Get("item_condition_id"))
	p.ShippingPayerID = splitIntParam(q.Get("shipping_payer_id"))
	p.SizeID = splitIntParam(q.Get("size_id"))
	p.ColorID = splitIntParam(q.Get("color_id"))

	return p
}

func BuildSearchURL(p SearchParams) string {
	q := url.Values{}
	if p.Keyword != "" {
		q.Set("keyword", p.Keyword)
	}
	if p.ExcludeKeyword != "" {
		q.Set("exclude_keyword", p.ExcludeKeyword)
	}
	if s, ok := reverseSortMap[p.Sort]; ok {
		q.Set("sort", s)
	}
	if o, ok := reverseOrderMap[p.Order]; ok {
		q.Set("order", o)
	}
	for _, st := range p.Status {
		if s, ok := reverseStatusMap[st]; ok {
			q.Add("status", s)
		}
	}
	if p.PriceMin > 0 {
		q.Set("price_min", strconv.Itoa(p.PriceMin))
	}
	if p.PriceMax > 0 {
		q.Set("price_max", strconv.Itoa(p.PriceMax))
	}
	if len(p.ItemConditionID) > 0 {
		q.Set("item_condition_id", joinInts(p.ItemConditionID))
	}
	if len(p.ColorID) > 0 {
		q.Set("color_id", joinInts(p.ColorID))
	}
	if len(p.SizeID) > 0 {
		q.Set("size_id", joinInts(p.SizeID))
	}
	return "https://jp.mercari.com/en/search?" + q.Encode()
}

func splitParam(val string) []string {
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			result = append(result, p)
		}
	}
	return result
}

func splitIntParam(val string) []int {
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	result := make([]int, 0, len(parts))
	for _, p := range parts {
		if v, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
			result = append(result, v)
		}
	}
	return result
}

func joinInts(vals []int) string {
	s := make([]string, len(vals))
	for i, v := range vals {
		s[i] = fmt.Sprintf("%d", v)
	}
	return strings.Join(s, ",")
}
