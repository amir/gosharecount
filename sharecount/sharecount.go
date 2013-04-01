package sharecount

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"appengine"
	"appengine/urlfetch"
)

type Result struct {
	Service string
	Count   int
}

var (
	context         appengine.Context
	googlePlusOneRE = regexp.MustCompile(`<div id="aggregateCount" class="V1">([0-9]+)</div>`)
)

func init() {
	http.HandleFunc("/", handler)
}

func query(url string) (results []Result) {
	c := make(chan Result)

	go func() { c <- twitter(url) }()
	go func() { c <- facebook(url) }()
	go func() { c <- linkedin(url) }()
	go func() { c <- google(url) }()

	timeout := time.After(100 * time.Millisecond)
	for i := 0; i < 4; i++ {
		select {
		case result := <-c:
			results = append(results, result)
		case <-timeout:
			fmt.Println("timed out")
			return
		}
	}
	return
}

func fetchUrl(url string) (body []byte, err error) {
	client := urlfetch.Client(context)
	response, err := client.Get(url)
	if err != nil {
		return
	}

	defer response.Body.Close()
	body, err = ioutil.ReadAll(response.Body)
	if err != nil {
		return
	}

	return
}

func twitter(url string) (result Result) {
	result.Service = "twitter"
	result.Count = -1

	endpoint := fmt.Sprintf("http://urls.api.twitter.com/1/urls/count.json?url=%s", url)
	body, err := fetchUrl(endpoint)
	if err != nil {
		context.Warningf("Error in getting response from Twitter: %s", err.Error())
		return
	}

	type twitterResult struct {
		Count int
	}
	var t twitterResult

	err = json.Unmarshal(body, &t)
	if err != nil {
		context.Warningf("Error in parsing response from Twitter: %s", err.Error())
		return
	}

	result.Count = t.Count
	return
}

func facebook(url string) (result Result) {
	result.Service = "facebook"
	result.Count = -1

	endpoint := fmt.Sprintf("http://api.ak.facebook.com/restserver.php?v=1.0&method=links.getStats&format=json&urls=%s", url)
	body, err := fetchUrl(endpoint)
	if err != nil {
		context.Warningf("Error in getting response from Facebook: %s", err.Error())
		return
	}

	type facebookResult struct {
		Total_count int
	}
	var f []facebookResult

	err = json.Unmarshal(body, &f)
	if err != nil {
		context.Warningf("Error in parsing response from Facebook: %s", err.Error())
		return
	}

	result.Count = f[0].Total_count
	return
}

func google(url string) (result Result) {
	result.Service = "googleplusone"
	result.Count = -1

	endpoint := fmt.Sprintf("https://plusone.google.com/_/+1/fastbutton?url=%s", url)
	body, err := fetchUrl(endpoint)
	if err != nil {
		context.Warningf("Error in getting response from Google: %s", err.Error())
		return
	}

	for _, m := range googlePlusOneRE.FindAllSubmatch(body, -1) {
		count, err := strconv.Atoi(string(m[1]))
		if err == nil {
			result.Count = count
		}
	}

	return
}

func linkedin(url string) (result Result) {
	result.Service = "linkedin"
	result.Count = -1
	endpoint := fmt.Sprintf("http://www.linkedin.com/countserv/count/share?format=json&url=%s", url)
	body, err := fetchUrl(endpoint)
	if err != nil {
		context.Warningf("Error in getting response from LinkedIn: %s", err.Error())
		return
	}

	type linkedinResult struct {
		Count int
	}
	var l linkedinResult

	err = json.Unmarshal(body, &l)
	if err != nil {
		context.Warningf("Error in parsing response from LinkedIn: %s", err.Error())
		return
	}

	result.Count = l.Count
	return
}

func handler(w http.ResponseWriter, r *http.Request) {
	context = appengine.NewContext(r)
	w.Header().Set("Content-Type", "application/json")

	url := url.QueryEscape(r.FormValue("url"))
	if url == "" {
		w.Write([]byte("{\"error\": \"The parameter url is required\"}"))
	} else {
		results := query(url)
		value, _ := json.Marshal(results)
		w.Write(value)
	}
}
