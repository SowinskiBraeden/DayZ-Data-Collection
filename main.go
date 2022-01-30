package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/jmcvetta/napping"
	"github.com/joho/godotenv"
)

type PositionHistory struct {
	Time string
	Pos  []float64
}

type Query struct {
	Gamertag         string
	PlayerID         string
	Time             string
	Pos              []float64
	PosHistory       []PositionHistory
	connectionStatus string
}

type Players struct {
	Players []Query
}

var players Players

func substr(input string, start int, length int) string {
	asRunes := []rune(input)

	if start >= len(asRunes) {
		return ""
	}

	if start+length > len(asRunes) {
		length = len(asRunes) - start
	}

	return string(asRunes[start : start+length])
}

func getEnvVar(key string) string {
	err := godotenv.Load(".env")

	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	return os.Getenv(key)
}

var logFlags [14]string = [14]string{
	"disconnected",
	") placed ",
	"connected",
	"hit by",
	"regained consciousnes",
	"is unconscious",
	"killed by",
	")Built ",
	") folded",
	")Player SurvivorBase",
	") died.",
	") committed suicide",
	")Dismantled",
	") bled",
}

// Download Raw Logs off Nitrado
func getRawLogs() {
	params := "file=" + url.QueryEscape("/games/ni5350965_2/noftp/dayzxb/config/DayZServer_X1_x64.ADM")
	url := fmt.Sprintf("https://api.nitrado.net/services/"+getEnvVar("SERVER_ID")+"/gameservers/file_server/download?%s", params)

	s := napping.Session{}
	h := &http.Header{}
	h.Set("Authorization", getEnvVar("AUTH_KEY"))
	s.Header = h

	resp, err := s.Get(url, nil, nil, nil)
	if err != nil {
		log.Fatal(err)
	}

	byt := []byte(resp.RawText())

	var data map[string]interface{}

	if err := json.Unmarshal(byt, &data); err != nil {
		panic(err)
	}
	targetURL := data["data"].(map[string]interface{})["token"].(map[string]interface{})["url"].(string)

	// Create the file
	out, err := os.Create("./output/logs.ADM")
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	// Get the data
	newResp, err := http.Get(targetURL)
	if err != nil {
		log.Fatal(err)
	}
	defer newResp.Body.Close()

	// Write the body to log file
	_, err = io.Copy(out, newResp.Body)
	if err != nil {
		log.Fatal(err)
	}
}

// Convert Raw Logs into cleaned logs (only positional data logs)
func cleanLogs() {
	file, err := os.Open("./output/logs.ADM")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	sc := bufio.NewScanner(file)

	// Create the file
	out, err := os.Create("./output/clean.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	// Create a writer
	w := bufio.NewWriter(out)

	for sc.Scan() {
		if strings.Contains(sc.Text(), ` | Player "`) {
			flagCheck := false
			for i := range logFlags {
				if strings.Contains(sc.Text(), logFlags[i]) == true {
					flagCheck = true
					break
				}
			}
			if flagCheck != true {
				w.WriteString(sc.Text() + "\n")
			}
		}
	}

	// Very important to invoke after writing a large number of lines
	w.Flush()
}

// Collect
func collectPlayerData() {
	file, err := os.Open("./output/clean.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	sc := bufio.NewScanner(file)
	for sc.Scan() {
		// Get Player Gamertag
		endPlayer := strings.Index(sc.Text(), `(`) - 2
		playerName := substr(sc.Text(), 19, (endPlayer - 19))

		// Get Player ID
		beginID := strings.Index(sc.Text(), "(id=") + 4
		endID := strings.Index(sc.Text(), "pos=<") - 1
		playerID := substr(sc.Text(), beginID, (endID - beginID))

		// Get Player pos
		beginPos := strings.Index(sc.Text(), "pos=<") + 5
		endPos := len(sc.Text()) - 2
		playerPos := strings.Split(substr(sc.Text(), beginPos, (endPos-beginPos)), ", ")
		var posFloatArr []float64
		for i, pos := range playerPos {
			f, err := strconv.ParseFloat(pos, 64)
			if err != nil {
				log.Fatal(err)
			}
			posFloatArr[i] = f
		}

		// Get Log Time
		logTime := substr(sc.Text(), 0, 8) + " EST"

		var query Query
		query.Gamertag = playerName
		query.PlayerID = playerID
		query.Time = logTime
		query.Pos = posFloatArr

		if len(players.Players) == 0 {
			players.Players = append(players.Players, query)
		} else {
			for i := range players.Players {
				if players.Players[i].Gamertag == playerName {
					// Updates Existing player data
					for j := range players.Players[i].PosHistory {
						var positionHistory PositionHistory
						positionHistory.Time = players.Players[i].PosHistory[j].Time
						positionHistory.Pos = players.Players[i].PosHistory[j].Pos
						query.PosHistory = append(query.PosHistory, positionHistory)
					}
					var positionHistory PositionHistory
					positionHistory.Time = players.Players[i].Time
					positionHistory.Pos = players.Players[i].Pos
					query.PosHistory = append(query.PosHistory, positionHistory)
					players.Players = players.Players[:i+copy(players.Players[i:], players.Players[i+1:])]
					break
				}
			}
			// Logs new player data
			players.Players = append(players.Players, query)
		}
	}
}

// Check raw logs for connected or disconnected messages
func activeStatus() {
	file, err := os.Open("./output/logs.ADM")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	sc := bufio.NewScanner(file)

	for sc.Scan() {
		if strings.Contains(sc.Text(), ` | Player "`) {
			var status string
			if strings.Contains(sc.Text(), "connected") {
				status = "Online"
			} else if strings.Contains(sc.Text(), "disconnected") {
				status = "Offline"
			}
			// Get Player ID
			beginID := strings.Index(sc.Text(), "(id=") + 4
			endID := strings.Index(sc.Text(), ")")
			playerID := substr(sc.Text(), beginID, (endID - beginID))

			foundPlayerAndUpdated := false
			for i := range players.Players {
				if players.Players[i].PlayerID == playerID {
					players.Players[i].connectionStatus = status
					foundPlayerAndUpdated = true
				}
			}

			if !foundPlayerAndUpdated {
				// Get Player Gamertag
				var playerName string
				if strings.Contains(sc.Text(), "disconnected") {
					endPlayer := strings.Index(sc.Text(), `(`) - 2
					playerName = substr(sc.Text(), 19, (endPlayer - 19))
				} else {
					endPlayer := strings.Index(sc.Text(), `(`) - 15
					playerName = substr(sc.Text(), 19, (endPlayer - 19))
				}

				var query Query
				query.Gamertag = playerName
				query.PlayerID = playerID
				query.connectionStatus = status

				// Logs new player data
				players.Players = append(players.Players, query)
			}
		}
	}
}

func main() {
	getRawLogs()
	cleanLogs()
	collectPlayerData()
	activeStatus()

	jsonFile, _ := json.MarshalIndent(players, "", " ")

	_ = ioutil.WriteFile("./output/players.json", jsonFile, 0644)
}
