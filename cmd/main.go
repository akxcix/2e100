package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func main() {
	fmt.Println("Starting 2e100")

	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Welcome to 2e100"))
	})

	bingAPIKey := Azurekey
	genLangAPIKey := Geminikey

	r.Get("/search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		if query == "" {
			http.Error(w, "Query parameter 'query' is missing", http.StatusBadRequest)
			return
		}

		links, err := bingSearchEngineAPI(bingAPIKey, query)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to search: %v", err), http.StatusInternalServerError)
			return
		}

		contents, err := fetchSitesContent(links)
		if err != nil {
			http.Error(w, "Failed to fetch site contents", http.StatusInternalServerError)
			return
		}

		summary, err := summarizeContents(genLangAPIKey, contents[:2])
		if err != nil {
			http.Error(w, "Failed to summarize content", http.StatusInternalServerError)
			return
		}

		response := struct {
			Links   []string `json:"links"`
			Summary string   `json:"summary"`
		}{
			Links:   links,
			Summary: summary,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	http.ListenAndServe(":3000", r)
}

func bingSearchEngineAPI(apiKey, query string) ([]string, error) {
	endpoint := "https://api.bing.microsoft.com/v7.0/search"

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("q", query)
	req.URL.RawQuery = q.Encode()

	req.Header.Add("Ocp-Apim-Subscription-Key", apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		WebPages struct {
			Value []struct {
				URL string `json:"url"`
			} `json:"value"`
		} `json:"webPages"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	var links []string
	for _, v := range result.WebPages.Value {
		links = append(links, v.URL)
	}

	return links, nil
}

type siteContent struct {
	URL     string `json:"url"`
	Content string `json:"content"`
}

func fetchSitesContent(urls []string) ([]siteContent, error) {
	var contents []siteContent

	for _, url := range urls {
		resp, err := http.Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		content := siteContent{
			URL:     url,
			Content: string(body), // This is the raw HTML; parsing it is another challenge
		}
		contents = append(contents, content)
	}

	return contents, nil
}

type GenerateContentPayload struct {
	Contents []struct {
		Role  string `json:"role"`
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"contents"`
}

type ApiResponse struct {
	Text []struct {
		Text string `json:"text"`
	} `json:"text"`
}

func summarizeContents(apiKey string, contents []siteContent) (string, error) {
	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:generateContent?key=" + apiKey

	payload := GenerateContentPayload{
		Contents: []struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		}{
			{
				Role: "user",
				Parts: []struct {
					Text string `json:"text"`
				}{
					{Text: "Summarize the following web pages as one. Be as much factual as possible."},
				},
			},
		},
	}

	for _, content := range contents {
		payload.Contents[0].Parts = append(payload.Contents[0].Parts, struct {
			Text string `json:"text"`
		}{Text: content.Content})
	}

	// for _, content := range contents {
	// 	payload.Contents = append(payload.Contents, struct {
	// 		Role  string `json:"role"`
	// 		Parts []struct {
	// 			Text string `json:"text"`
	// 		} `json:"parts"`
	// 	}{
	// 		Role: "user",
	// 		Parts: []struct {
	// 			Text string `json:"text"`
	// 		}{
	// 			{Text: content.Content},
	// 		},
	// 	})
	// }

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshaling payload: %v", err)
		return "", err
	}

	log.Printf("Payload to API: %s", string(jsonPayload))

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error making request: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return "", err
	}

	log.Printf("Response from API: %s", string(body))

	var apiResp ApiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		log.Printf("Error unmarshaling response: %v", err)
		return "", err
	}

	if len(apiResp.Text) > 0 {
		return apiResp.Text[0].Text, nil
	}

	return "", fmt.Errorf("no summary returned from the API")
}
