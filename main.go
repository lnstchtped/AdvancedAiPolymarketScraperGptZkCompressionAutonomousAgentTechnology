package main

import (
	"encoding/json"
	"fmt"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const ticker = "who-will-hbo-doc-identify-as-satoshi"

var buildID = ""

func main() {
	err := ui.Init()
	if err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	defer ui.Close()

	buildID, err = getBuildID()
	if err != nil {
		log.Fatalf("failed to get build id: %v", err)
	}

	table := widgets.NewTable()
	table.Title = "Who is Satoshi?"
	table.Rows = [][]string{
		{"Name", "Price"},
	}

	_, termHeight := ui.TerminalDimensions()

	table.TextStyle = ui.NewStyle(ui.ColorWhite)
	table.RowSeparator = true
	table.FillRow = true
	table.SetRect(0, 0, 50, termHeight)

	go func() {
		for {
			prices, err := fetch()
			if err != nil {
				log.Printf("Failed to fetch data: %v", err)
				return
			}

			var rows [][]string
			rows = append(rows, []string{"Name", " Probability"})
			for _, p := range prices {
				rows = append(rows, []string{p.Name, fmt.Sprintf(" %.2f%%", p.Price*100)})
			}
			table.Rows = rows

			ui.Render(table)

			time.Sleep(1 * time.Minute)
		}
	}()

	uiEvents := ui.PollEvents()

	for {
		select {
		case e := <-uiEvents:
			switch e.ID {
			case "<C-c>":
				return
			case "<Resize>":
				payload := e.Payload.(ui.Resize)
				table.SetRect(0, 0, 50, payload.Height)
				ui.Render(table)
			}
		}
	}
}

func getBuildID() (string, error) {
	resp, err := http.Get("https://polymarket.com/markets/crypto")
	if err != nil {
		return "", fmt.Errorf("failed to get url: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if !strings.Contains(string(body), `"buildId":"`) {
		return "", fmt.Errorf("build id not found")
	}

	buildID = strings.Split(strings.Split(string(body), `"buildId":"`)[1], `"`)[0]

	return buildID, nil
}

type response struct {
	PageProps struct {
		DehydratedState struct {
			Queries []struct {
				State struct {
					Data []struct {
						Ticker  string `json:"ticker,omitempty"`
						Markets []struct {
							OutcomePrices  []string `json:"outcomePrices"`
							GroupItemTitle string   `json:"groupItemTitle"`
						} `json:"markets,omitempty"`
					} `json:"data"`
				} `json:"state"`
			} `json:"queries"`
		} `json:"dehydratedState"`
	} `json:"pageProps"`
}

type price struct {
	Name  string
	Price float64
}

func fetch() ([]price, error) {
	resp, err := http.Get(fmt.Sprintf("https://polymarket.com/_next/data/%s/en/markets/crypto.json?slug=crypto", buildID))
	if err != nil {
		return nil, fmt.Errorf("failed to get url: %w", err)
	}

	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		buildID, err = getBuildID()
		if err != nil {
			return nil, fmt.Errorf("failed to get build id: %w", err)
		}
		return fetch()
	default:
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var r response
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(r.PageProps.DehydratedState.Queries) == 0 {
		return nil, fmt.Errorf("no queries found")
	}

	var prices []price

	for _, data := range r.PageProps.DehydratedState.Queries[0].State.Data {
		if data.Ticker == ticker {
			for _, market := range data.Markets {
				yesPriceFloat, err := strconv.ParseFloat(market.OutcomePrices[0], 64)
				if err != nil {
					return nil, fmt.Errorf("failed to parse price: %w", err)
				}

				prices = append(prices, price{
					Name:  strings.TrimSpace(market.GroupItemTitle),
					Price: yesPriceFloat,
				})
			}
		}
	}

	sort.Slice(prices, func(i, j int) bool {
		return prices[i].Price > prices[j].Price
	})

	if prices == nil {
		return nil, fmt.Errorf("no prices found")
	}

	return prices, nil
}
