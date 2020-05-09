package main

import (
  "fmt"
  "io/ioutil"
  "net/http"
  "os"
  "strings"
)

const cSearchURLMissingKeyword string = "https://store.steampowered.com/search/suggest?term=%s&f=games&cc=US"

func fetchGame(keywords []string) {
  searchURL := fmt.Sprintf(cSearchURLMissingKeyword, strings.Join(keywords, "+"))
  resp, err := http.Get(searchURL)
  if err != nil {
    fmt.Printf("Error fetching keywords \"%s\" (err = %+v)\n", strings.Join(keywords, " "), err)
    return
  }

  defer resp.Body.Close()
  body, err := ioutil.ReadAll(resp.Body)
  if err != nil {
    fmt.Printf("Error fetching keywords \"%s\" (err = %+v)\n", strings.Join(keywords, " "), err)
    return
  }

  // The result is an HTML file where each game is inside an anchor <a>.
  fmt.Printf("Got %s\n", body)
}

func main() {
  if len(os.Args) == 1 {
    fmt.Printf("Usage: %s <search_keyword>+\n", os.Args[0])
    return
  }

  fetchGame(os.Args[1:])
}
