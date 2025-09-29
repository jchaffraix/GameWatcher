package main

import (
  "encoding/csv"
  "errors"
  "fmt"
  "flag"
  "io"
  "os"
  "sort"
  "strconv"
  "strings"
  "sync"
)

// TODO: All those common structs need to go to a shared file.
type SteamInfo struct {
  // Either id (regular game) or bundleId (for bundles) will be ever set.
  id int
  bundleId int

  // Game price may be -1 if none was found (unreleased games).
  price float32
}

type FanaticalInfo struct {
  // Game price may be -1 if the game is not listed.
  price float32

  // Slug is what is used to build the fanatical URL.
  // Empty if the game is unreleased or inexistent.
  slug string
}

type GreenManGamingInfo struct {
  // Game price may be -1 if the game is not listed.
  price float32

  // Path to the game.
  path string
}

type HumbleBundleInfo struct {
  // Game price may be -1 if the game is not listed.
  price float32

  // Path to the game.
  path string
}

type LoadedInfo struct {
  price float32

  // Full URL to game.
  url string
}

type Game struct {
  // The name of the game in Steam.
  // This is used by the Fanatical backend to associate result (there is no ID we can use).
  name string

  minPrice float32
  backend string

  // Backend-specific info.
  steam SteamInfo
  fanatical FanaticalInfo
  gmg GreenManGamingInfo
  hb HumbleBundleInfo
  loaded LoadedInfo
}

func (g Game) url() string {
  if g.backend == "steam" {
    return g.steamURL()
  } else if g.backend == "fanatical" {
    return g.fanaticalURL()
  } else if g.backend == "gmg" {
    return g.greenManGamingURL()
  } else if g.backend == "loaded" {
    return g.loaded.url
  } else {
    if g.backend != "humblebundle" {
      panic(fmt.Sprintf("Unknown backend \"%s\"", g.backend))
    }
    return g.humbleBundleURL()
  }
}

func (g Game) steamURL() string {
  if g.steam.bundleId != 0 && g.steam.id != 0 {
    panic(fmt.Sprintf("Game is both a regular game and a bundle: %+v", g))
  }

  if g.steam.bundleId != 0 {
    return fmt.Sprintf("https://store.steampowered.com/bundle/%d", g.steam.bundleId)
  }

  return fmt.Sprintf("https://store.steampowered.com/app/%d", g.steam.id)
}

func (g Game) fanaticalURL() string {
  if g.fanatical.slug == "" {
    panic(fmt.Sprintf("Game doesn't have a fanatical slug: %+v", g))
  }

  return fmt.Sprintf("https://www.fanatical.com/en/game/%s", g.fanatical.slug)
}

func (g Game) greenManGamingURL() string {
  if g.gmg.path == "" {
    panic(fmt.Sprintf("Game doesn't have a GreenManGaming path: %+v", g))
  }

  return fmt.Sprintf("https://www.greenmangaming.com%s", g.gmg.path)
}

func (g Game) humbleBundleURL() string {
  if g.hb.path == "" {
    panic(fmt.Sprintf("Game doesn't have a HumbleBundle path: %+v", g))
  }

  // path contains a leading '/' for HumbleBundle.
  return fmt.Sprintf("https://www.humblebundle.com/store%s", g.hb.path)
}

type gameCriteria struct {
  name string
  targetPrice float32
}

func fetchAndFillGame(criteria gameCriteria) (error, *Game) {
  if debugFlag {
    fmt.Println("Fetching", criteria.name)
  }

  err, game := SearchGameOnSteam(criteria.name)
  if err != nil {
    return err, nil
  }

  if game.steam.price == -1 {
    if debugFlag {
      fmt.Printf("Ignoring unreleased game \"%s\" (from non-steam backend)\n", game.name)
    }
    return nil, game
  }

  err = FillFanaticalInfo(game)
  if err != nil {
    return err, nil
  }

  err = FillHumbleBundleInfo(game)
  if err != nil {
    return err, nil
  }

  err = FillGreenManGamingInfo(game)
  if err != nil {
    return err, nil
  }

  err = FillLoadedInfo(game)
  if err != nil {
    return err, nil
  }

  if debugFlag {
    fmt.Println("Done Fetching", criteria.name)
  }
  return nil, game
}

func gameWorker(c chan gameCriteria, output *Output) {
  defer output.wg.Done()
  for criteria := range(c) {
    err, game := fetchAndFillGame(criteria)
    if err != nil {
      fmt.Fprintf(os.Stderr, "Error fetching game \"%s\" (err = %+v)\n", criteria.name, err)
      continue
    }

    if game == nil {
      fmt.Fprintf(os.Stderr, "No matches for \"%s\"\n", criteria.name)
      continue
    }

    fillMinPrice(game)
    splitGameOnCriteria(*game, criteria.targetPrice, output)

    if debugFlag {
      fmt.Printf("Done for \"%s\", final game: %+v\n", criteria.name, *game)
    }
  }
}

func fillMinPrice(game *Game) {
  // Preference is steam, fanatical, HumbleBundle (hb), GreenManGaming (gmg), loaded.
  game.minPrice = game.steam.price
  game.backend = "steam"

  if game.fanatical.price > 0 && game.fanatical.price < game.minPrice {
    game.minPrice = game.fanatical.price
    game.backend = "fanatical"
  } 

  if game.hb.price > 0 && game.hb.price < game.minPrice {
    game.minPrice = game.hb.price
    game.backend = "humblebundle"
  }

  if game.gmg.price > 0 && game.gmg.price < game.minPrice {
    game.minPrice = game.gmg.price
    game.backend = "gmg"
  }

  if game.loaded.price > 0 && game.loaded.price < game.minPrice {
    game.minPrice = game.loaded.price
    game.backend = "loaded"
  }
}

func splitGameOnCriteria(game Game, targetPrice float32, output *Output) {
  output.m.Lock()
  defer output.m.Unlock()

  if game.steam.price == -1 {
    output.unreleasedGames = append(output.unreleasedGames, game)
    return
  }

  // Simple price point right now.
  if game.minPrice < targetPrice {
    if debugFlag {
      fmt.Fprintf(os.Stdout, "Game \"%s\" with price = %v (backend = \"%s\") matched targetPrice = %v\n", game.name, game.minPrice, game.backend, targetPrice)
    }
    output.matchingGames = append(output.matchingGames, game)
    return
  }

  if debugFlag {
    fmt.Fprintf(os.Stdout, "Game \"%s\" with price = %v (backend = \"%s\") was over targetPrice = %v\n", game.name, game.minPrice, game.backend, targetPrice)
  }
  output.otherGames = append(output.otherGames, game)
}

var parallelism int = 10

type Output struct {
  // Channels don't work here as we read from
  // one of the channels at a time, leading to
  // deadlocks (main thread is waiting on new input
  // on one channel when all the worker threads are
  // waiting for their write to be acknoweldged
  // on the other channels). We *could* create
  // some buffer channels but that would be pretty
  // equivalent to this as they would have to be
  // sized after the total number of games to fetch.
  //
  // TODO: Think about this more. Maybe we can
  // figure out how to use a single output channel
  // (maybe by annotating the game struct?).
  unreleasedGames []Game
  matchingGames []Game
  otherGames []Game
  m sync.Mutex

  wg sync.WaitGroup
}

// ByPriceThenName implements sort.Interface for []game.
type ByPriceThenName []Game
func (a ByPriceThenName) Len() int { return len(a) }
func (a ByPriceThenName) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByPriceThenName) Less(i, j int) bool {
  iPrice := a[i].minPrice
  jPrice := a[j].minPrice
  if (iPrice < jPrice) {
    return true;
  }

  if (iPrice > jPrice) {
    return false;
  }

  return a[i].name < a[j].name
}

func newOutput() Output {
  return Output{[]Game{}, []Game{}, []Game{}, sync.Mutex{}, sync.WaitGroup{}}
}

func readGamesFromFiles(fileName string) ([]gameCriteria, error) {
  // Check that the file exist and is valid.
  // For some reason, os.Open doesn't return an error when opening a directory.
  stats, err := os.Stat(fileName)
  if err != nil {
    return []gameCriteria{}, err
  }
  if stats.IsDir() {
    return []gameCriteria{}, errors.New("File is a directory")
  }

  file, err := os.Open(fileName)
  if err != nil {
    return []gameCriteria{}, err
  }

    // Ensure that we close c to avoid deadlocks in case of errors.
  defer file.Close()

  uniqueGameNames := make(map[string] bool)
  criteria := make([]gameCriteria, 0)
  csvReader := csv.NewReader(file)
  for {
    records, err := csvReader.Read()
    // Handle EOF as a special error.
    if err == io.EOF {
      break
    }

    // We want to allow an optional targetPrice.
    // This means that we ignore ErrFieldCount errors by looking at the presence of `records`.
    if err != nil && records == nil {
      return []gameCriteria{}, err
    }
    if len(records) == 0 {
      panic("Invalid CSV file, no record on line")
    }
    gameName := records[0]

    // Check if the name is unique.
    _, exists := uniqueGameNames[gameName]
    if exists {
      panic(fmt.Sprintf("Duplicated name \"%s\"", gameName))
    }
    uniqueGameNames[gameName] = true

    // Start with our default and override it if specified.
    targetPrice := cDefaultTargetPrice
    if len(records) == 2 {
      tmp, err := strconv.ParseFloat(strings.TrimSpace(records[1]), /*bitSize=*/32)
      if err != nil {
        return []gameCriteria{}, err
      }
      targetPrice = float32(tmp)
    }
    criteria = append(criteria, gameCriteria{gameName, targetPrice})
  }
  return criteria, nil
}

func feedGamesFromFile(fileName string, c chan gameCriteria) error {
  gameCriteria, err := readGamesFromFiles(fileName)
  if err != nil {
    return err
  }

  for _, gameCriterium := range gameCriteria {
    c <- gameCriterium
  }

  return nil
}

func feedGamesFromFlag(games string, c chan gameCriteria) {
  tokens := strings.Split(games, ",")
  idx := 0
  for ; idx < len(tokens); idx++ {
    gameName := tokens[idx];
    // Start with our default and override it if specified.
    targetPrice := cDefaultTargetPrice
    if idx < len(tokens) - 1 {
      lookAheadToken := tokens[idx + 1]
      tmp, err := strconv.ParseFloat(lookAheadToken, /*bitSize=*/32)
      if err == nil {
        targetPrice = float32(tmp)
        // Skip next token as it was an optional price.
        idx += 1
      }
    }
    c <- gameCriteria{gameName, targetPrice}
  }
}

func main() {
  flag.BoolVar(&debugFlag, "debug", false, "Enable debug statements")
  flag.StringVar(&gamesFlag, "games", "", "Commad separated list of games to fetch")
  flag.StringVar(&fileFlag, "file", "", "File containing a CSV list of games")
  flag.Parse()

  if gamesFlag == "" && fileFlag == "" || (gamesFlag != "" && fileFlag != "") {
    fmt.Printf("Usage: main [-debug] [-file <file>] [-games game1,7,game2,game3]\n\n\nEither -file or -games must be set, but not both.\n\n<file> contains one game name per line along with a potential target price divided by ','\nExample: Foobar, 10\n")
    return
  }

  InitFanatical()

  c := make(chan gameCriteria, parallelism)
  output := newOutput()
  output.wg.Add(parallelism)

  // Start the workers.
  for i := 0; i < parallelism; i++ {
      go gameWorker(c, &output)
  }

  // Feed the games as they are read.
  if (fileFlag != "") {
    err := feedGamesFromFile(fileFlag, c)
    if err != nil {
      fmt.Fprintf(os.Stderr, "Error processing file=%s (err = %+v)\n", fileFlag, err)
      return
    }
  } else {
    feedGamesFromFlag(gamesFlag, c)
  }

  // Make sure the channel is closed.
  close(c)
  output.wg.Wait()

  // Sort the output by price, then name.
  // This gives a stable sort for quickly assessing games.
  sort.Sort(ByPriceThenName(output.unreleasedGames))
  sort.Sort(ByPriceThenName(output.matchingGames))
  sort.Sort(ByPriceThenName(output.otherGames))

  if len(output.unreleasedGames) > 0 {
    fmt.Fprintf(os.Stdout, "==================================================\n")
    fmt.Fprintf(os.Stdout, "============== Unreleased games ==================\n")
    fmt.Fprintf(os.Stdout, "==================================================\n")
    for _, game := range output.unreleasedGames {
      fmt.Fprintf(os.Stdout, "%s - %s \n", game.name, game.steamURL())
    }
    fmt.Fprintf(os.Stdout, "\n\n")
  }

  if len(output.matchingGames) > 0 {
    fmt.Fprintf(os.Stdout, "==================================================\n")
    fmt.Fprintf(os.Stdout, "============== Games under target ================\n")
    fmt.Fprintf(os.Stdout, "==================================================\n")
    for _, game := range output.matchingGames {
      fmt.Fprintf(os.Stdout, "%s: $%.2f - %s \n", game.name, game.minPrice, game.url())
    }
    fmt.Fprintf(os.Stdout, "\n\n")
  }

  fmt.Fprintf(os.Stdout, "==================================================\n")
  fmt.Fprintf(os.Stdout, "=============== Games over target ================\n")
  fmt.Fprintf(os.Stdout, "==================================================\n")
  for _, game := range output.otherGames {
    fmt.Fprintf(os.Stdout, "%s: $%.2f - %s\n", game.name, game.minPrice, game.url())
  }
  fmt.Fprintf(os.Stdout, "==================================================\n")
}
