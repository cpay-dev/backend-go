package checkoutsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const quoteTimeout = 5 * time.Second

var binanceUSDPairs = map[string]string{
	"ETH":   "ETHUSDT",
	"BNB":   "BNBUSDT",
	"POL":   "POLUSDT",
	"MATIC": "MATICUSDT",
	"OP":    "OPUSDT",
	"ARB":   "ARBUSDT",
}

type quoteSource struct {
	name  string
	quote func(context.Context, string) (float64, error)
}

func settlementAmountForToken(ctx context.Context, fiatAmount float64, currency, chainName, symbol string) (float64, error) {
	if fiatAmount <= 0 {
		return 0, errors.New("amount must be positive")
	}

	currency = strings.ToUpper(strings.TrimSpace(currency))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if currency != "USD" {
		return 0, fmt.Errorf("checkout currency %s cannot be quoted for %s on %s", currency, symbol, strings.TrimSpace(chainName))
	}
	if isUSDStableToken(symbol) {
		return fiatAmount, nil
	}

	price, err := tokenUSDPrice(ctx, symbol)
	if err != nil {
		return 0, err
	}
	if price <= 0 {
		return 0, fmt.Errorf("invalid %s/USD price", symbol)
	}
	return fiatAmount / price, nil
}

func isUSDStableToken(symbol string) bool {
	switch strings.ToUpper(strings.TrimSpace(symbol)) {
	case "USDC", "USDT", "USDT0", "DAI":
		return true
	default:
		return false
	}
}

func tokenUSDPrice(ctx context.Context, symbol string) (float64, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "HYPE" {
		return firstAvailableTokenUSDPrice(ctx, symbol, []quoteSource{
			{name: "Hyperliquid", quote: hyperliquidMidPrice},
			{name: "Bybit linear", quote: func(ctx context.Context, symbol string) (float64, error) {
				return bybitTickerPrice(ctx, "linear", symbol+"USDT", symbol)
			}},
			{name: "Bybit spot", quote: func(ctx context.Context, symbol string) (float64, error) {
				return bybitTickerPrice(ctx, "spot", symbol+"USDT", symbol)
			}},
		})
	}

	pair, ok := binanceUSDPairs[symbol]
	if !ok {
		return 0, fmt.Errorf("no USD quote source for %s", symbol)
	}
	return binanceTickerPrice(ctx, pair, symbol)
}

func firstAvailableTokenUSDPrice(ctx context.Context, symbol string, sources []quoteSource) (float64, error) {
	var failures []string
	for _, source := range sources {
		price, err := source.quote(ctx, symbol)
		if err == nil && price > 0 {
			return price, nil
		}
		if err != nil {
			failures = append(failures, source.name+": "+err.Error())
			continue
		}
		failures = append(failures, source.name+": invalid non-positive price")
	}
	return 0, fmt.Errorf("no %s/USD quote available (%s)", symbol, strings.Join(failures, "; "))
}

func hyperliquidMidPrice(ctx context.Context, symbol string) (float64, error) {
	var mids map[string]string
	err := fetchQuoteJSON(ctx, http.MethodPost, "https://api.hyperliquid.xyz/info", "application/json", strings.NewReader(`{"type":"allMids"}`), &mids)
	if err != nil {
		return 0, fmt.Errorf("failed to quote %s/USD from Hyperliquid: %w", symbol, err)
	}

	price, err := parsePositivePrice(mids[symbol])
	if err != nil {
		return 0, fmt.Errorf("missing %s/USD quote in Hyperliquid allMids response", symbol)
	}
	return price, nil
}

func bybitTickerPrice(ctx context.Context, category, pair, symbol string) (float64, error) {
	endpoint := "https://api.bybit.com/v5/market/tickers?category=" + url.QueryEscape(category) + "&symbol=" + url.QueryEscape(pair)
	var payload struct {
		RetCode int    `json:"retCode"`
		RetMsg  string `json:"retMsg"`
		Result  struct {
			List []struct {
				LastPrice  string `json:"lastPrice"`
				IndexPrice string `json:"indexPrice"`
			} `json:"list"`
		} `json:"result"`
	}
	if err := fetchQuoteJSON(ctx, http.MethodGet, endpoint, "", nil, &payload); err != nil {
		return 0, fmt.Errorf("failed to quote %s/USD from Bybit %s: %w", symbol, category, err)
	}
	if payload.RetCode != 0 {
		return 0, fmt.Errorf("failed to quote %s/USD from Bybit %s: %s", symbol, category, payload.RetMsg)
	}
	if len(payload.Result.List) == 0 {
		return 0, fmt.Errorf("missing %s/USD quote in Bybit %s ticker response", symbol, category)
	}

	rawPrice := strings.TrimSpace(payload.Result.List[0].IndexPrice)
	if rawPrice == "" {
		rawPrice = strings.TrimSpace(payload.Result.List[0].LastPrice)
	}
	price, err := parsePositivePrice(rawPrice)
	if err != nil {
		return 0, fmt.Errorf("missing %s/USD quote in Bybit %s ticker response", symbol, category)
	}
	return price, nil
}

func binanceTickerPrice(ctx context.Context, pair, symbol string) (float64, error) {
	endpoint := "https://api.binance.com/api/v3/ticker/price?symbol=" + url.QueryEscape(pair)
	var payload struct {
		Price string `json:"price"`
	}
	if err := fetchQuoteJSON(ctx, http.MethodGet, endpoint, "", nil, &payload); err != nil {
		return 0, fmt.Errorf("failed to quote %s/USD from Binance: %w", symbol, err)
	}

	price, err := parsePositivePrice(payload.Price)
	if err != nil {
		return 0, fmt.Errorf("missing %s/USD quote in Binance ticker response", symbol)
	}
	return price, nil
}

func fetchQuoteJSON(ctx context.Context, method, endpoint, contentType string, body io.Reader, dst any) error {
	ctx, cancel := context.WithTimeout(ctx, quoteTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("status %d", res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func parsePositivePrice(raw string) (float64, error) {
	price, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || price <= 0 {
		return 0, errors.New("price must be positive")
	}
	return price, nil
}
