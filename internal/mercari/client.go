package mercari

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ilyasaftr/mercari-price-tracking/domain"
)

const (
	searchURL      = "https://api.mercari.jp/v2/entities:search"
	itemAPI        = "https://api.mercari.jp/items/get"
	translationAPI = "https://api.mercari.jp/v2/itemtranslations"
	currencyAPI    = "https://api.mercari.jp/v2/getCurrencyConversionRate/country"
	itemBase       = "https://jp.mercari.com/en/item/"
	userAgent      = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36"
	pageSize       = 120
)

type Client struct {
	httpClient *http.Client
	dpop       *dpopGenerator
	rateMu     sync.Mutex
	usdRate    float64
	rateAt     time.Time
}

func NewClient() (*Client, error) {
	dpop, err := newDPoPGenerator()
	if err != nil {
		return nil, fmt.Errorf("create DPoP generator: %w", err)
	}

	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		dpop:       dpop,
	}, nil
}

func (c *Client) newRequest(ctx context.Context, method, dpopURL, fullURL string, body io.Reader) (*http.Request, error) {
	dpopToken, err := c.dpop.generate(method, dpopURL)
	if err != nil {
		return nil, fmt.Errorf("generate DPoP: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-Platform", "web")
	req.Header.Set("DPoP", dpopToken)
	return req, nil
}

func (c *Client) doJSON(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) Search(ctx context.Context, params domain.SearchParams) ([]domain.Item, error) {
	if params.ItemID != "" {
		item, err := c.GetItem(ctx, params.ItemID)
		if err != nil {
			return nil, err
		}
		if item == nil {
			return nil, nil
		}
		return []domain.Item{*item}, nil
	}

	body, err := json.Marshal(searchRequest{
		SearchSessionID: strings.ReplaceAll(c.dpop.deviceUUID, "-", ""),
		SearchCondition: searchCondition{
			Keyword:         params.Keyword,
			ExcludeKeyword:  params.ExcludeKeyword,
			Sort:            params.Sort,
			Order:           params.Order,
			Status:          params.Status,
			PriceMin:        params.PriceMin,
			PriceMax:        params.PriceMax,
			CategoryID:      emptySlice(params.CategoryID),
			BrandID:         emptySlice(params.BrandID),
			ItemConditionID: emptyIntSlice(params.ItemConditionID),
			ShippingPayerID: emptyIntSlice(params.ShippingPayerID),
			SizeID:          emptyIntSlice(params.SizeID),
			ColorID:         emptyIntSlice(params.ColorID),
			ItemTypes:       emptySlice(nil),
		},
		PageSize:            pageSize,
		IndexRouting:        "INDEX_ROUTING_UNSPECIFIED",
		ThumbnailTypes:      []string{},
		Source:              "BaseSerp",
		WithItemBrand:       true,
		WithItemPromotions:  true,
		WithItemSizes:       true,
		WithSuggestedItems:  true,
		WithOfferPricePromo: true,
		WithProductSuggest:  true,
		WithAuction:         true,
		UseDynamicAttribute: true,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodPost, searchURL, searchURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	var searchResp searchResponse
	if err := c.doJSON(req, &searchResp); err != nil {
		return nil, err
	}

	rate := c.getUSDRate(ctx)

	items := make([]domain.Item, 0, len(searchResp.Items))
	for _, ai := range searchResp.Items {
		item, err := toDomainItem(ai)
		if err != nil {
			continue
		}
		item.PriceUSD = toUSD(item.Price, rate)
		items = append(items, item)
	}

	return items, nil
}

func (c *Client) GetItem(ctx context.Context, itemID string) (*domain.Item, error) {
	fullURL := itemAPI + "?id=" + itemID + "&include_item_attributes=true&include_auction=true"

	req, err := c.newRequest(ctx, http.MethodGet, itemAPI, fullURL, nil)
	if err != nil {
		return nil, err
	}

	var result itemDetailResponse
	if err := c.doJSON(req, &result); err != nil {
		return nil, err
	}

	if result.Data.ID == "" {
		return nil, nil
	}

	item := detailToDomainItem(result.Data)

	if tr, err := c.getTranslation(ctx, itemID); err == nil && tr != nil {
		item.NameEN = tr.Name
		item.DescriptionEN = tr.Description
	}

	item.PriceUSD = toUSD(item.Price, c.getUSDRate(ctx))
	return &item, nil
}

func (c *Client) getTranslation(ctx context.Context, itemID string) (*itemTranslation, error) {
	dpopBase := translationAPI + "/" + itemID + "/translation"
	fullURL := dpopBase + "?name=" + itemID + "&sessionId=" + c.dpop.deviceUUID

	req, err := c.newRequest(ctx, http.MethodGet, dpopBase, fullURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept-Language", "en")

	var tr itemTranslation
	if err := c.doJSON(req, &tr); err != nil {
		return nil, err
	}
	return &tr, nil
}

func (c *Client) getUSDRate(ctx context.Context) float64 {
	c.rateMu.Lock()
	defer c.rateMu.Unlock()

	if c.usdRate > 0 && time.Since(c.rateAt) < 30*time.Minute {
		return c.usdRate
	}

	req, err := c.newRequest(ctx, http.MethodGet, currencyAPI, currencyAPI+"?country_code=US", nil)
	if err != nil {
		log.Printf("currency rate request: %v", err)
		return c.usdRate
	}

	var result struct {
		Rate float64 `json:"rate"`
	}
	if err := c.doJSON(req, &result); err != nil {
		log.Printf("fetch currency rate: %v", err)
		return c.usdRate
	}

	c.usdRate = result.Rate
	c.rateAt = time.Now()
	log.Printf("USD rate updated: %.5f", c.usdRate)
	return c.usdRate
}

func toUSD(yen int, rate float64) float64 {
	if rate <= 0 {
		return 0
	}
	return math.Round(float64(yen)*rate*100) / 100
}

func toDomainItem(ai apiItem) (domain.Item, error) {
	price, err := strconv.Atoi(ai.Price)
	if err != nil {
		return domain.Item{}, fmt.Errorf("parse price %q: %w", ai.Price, err)
	}

	created, _ := strconv.ParseInt(ai.Created, 10, 64)
	updated, _ := strconv.ParseInt(ai.Updated, 10, 64)

	thumbnail := ""
	if len(ai.Thumbnails) > 0 {
		thumbnail = ai.Thumbnails[0]
	}

	return domain.Item{
		ID:            ai.ID,
		Name:          ai.Name,
		Price:         price,
		Status:        ai.Status,
		ThumbnailURL:  thumbnail,
		ItemURL:       itemBase + ai.ID,
		SellerID:      ai.SellerID,
		CategoryID:    ai.CategoryID,
		ConditionID:   ai.ItemConditionID,
		ShippingPayer: ai.ShippingPayerID,
		CreatedAt:     time.Unix(created, 0),
		UpdatedAt:     time.Unix(updated, 0),
	}, nil
}

func detailToDomainItem(d itemDetail) domain.Item {
	priceInt, _ := d.Price.Int64()
	price := int(priceInt)
	created, _ := d.Created.Int64()
	updated, _ := d.Updated.Int64()

	photoURL := ""
	if len(d.Photos) > 0 {
		photoURL = d.Photos[0]
	}
	thumbnail := ""
	if len(d.Thumbnails) > 0 {
		thumbnail = d.Thumbnails[0]
	}

	return domain.Item{
		ID:            d.ID,
		Name:          d.Name,
		Description:   d.Description,
		Price:         price,
		Status:        d.Status,
		ThumbnailURL:  thumbnail,
		PhotoURL:      photoURL,
		ItemURL:       itemBase + d.ID,
		SellerID:      strconv.Itoa(d.Seller.ID),
		SellerName:    d.Seller.Name,
		CategoryID:    strconv.Itoa(d.ItemCategory.ID),
		ConditionID:   strconv.Itoa(d.ItemCondition.ID),
		Condition:     d.ItemCondition.Name,
		ShippingPayer: d.ShippingPayer.Name,
		NumLikes:      d.NumLikes,
		CreatedAt:     time.Unix(created, 0),
		UpdatedAt:     time.Unix(updated, 0),
	}
}

func emptySlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func emptyIntSlice(s []int) []int {
	if s == nil {
		return []int{}
	}
	return s
}
