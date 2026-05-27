package payment

import "strings"

type AllowedToken struct {
	Chain    string `json:"chain"`
	Symbol   string `json:"symbol"`
	Address  string `json:"address,omitempty"`
	Decimals int    `json:"decimals,omitempty"`
}

func TokenAllowed(allowed []AllowedToken, chainName, symbol, address string) bool {
	if len(allowed) == 0 {
		return true
	}
	chainName = strings.ToLower(strings.TrimSpace(chainName))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	address = strings.ToLower(strings.TrimSpace(address))
	for _, token := range allowed {
		if strings.ToLower(strings.TrimSpace(token.Chain)) != chainName {
			continue
		}
		if strings.ToUpper(strings.TrimSpace(token.Symbol)) != symbol {
			continue
		}
		if strings.TrimSpace(token.Address) == "" || address == "" {
			return true
		}
		if strings.ToLower(strings.TrimSpace(token.Address)) == address {
			return true
		}
	}
	return false
}
