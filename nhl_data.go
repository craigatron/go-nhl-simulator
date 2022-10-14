package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"

	"github.com/gocarina/gocsv"
)

const nhlURL = "https://statsapi.web.nhl.com/api/v1"
const currentSeason = "20222023"

type NHLSeasonJSON struct {
	Dates []NHLDateJSON `json:"dates"`
}

type NHLDateJSON struct {
	Date  string        `json:"date"`
	Games []NHLGameJSON `json:"games"`
}

type NHLGameJSON struct {
	GamePK   int64  `json:"gamePk"`
	GameType string `json:"gameType"`
	Status   struct {
		AbstractGameState string `json:"abstractGameState"`
	} `json:"status"`
	Teams struct {
		Away NHLGameTeamJSON `json:"away"`
		Home NHLGameTeamJSON `json:"home"`
	} `json:"teams"`
	Linescore struct {
		CurrentPeriodOrdinal string `json:"currentPeriodOrdinal"`
	} `json:"linescore"`
	Venue struct {
		Name string `json:"name"`
	} `json:"venue"`
}

type NHLGameTeamJSON struct {
	Score int `json:"score"`
	Team  struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
}

func GetNhlSeason() (NHLSeasonJSON, error) {
	res, err := http.Get(fmt.Sprintf("%s/schedule?season=%s&expand=schedule.linescore", nhlURL, currentSeason))
	if err != nil {
		return NHLSeasonJSON{}, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return NHLSeasonJSON{}, err
	}

	var seasonJSON NHLSeasonJSON
	err = json.Unmarshal(body, &seasonJSON)
	return seasonJSON, err
}

type NHLTeamsJSON struct {
	Teams []NHLTeamJSON `json:"teams"`
}

type NHLTeamJSON struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
	Active       bool   `json:"active"`
	Division     struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		Abbreviation string `json:"abbreviation"`
	} `json:"division"`
	Conference struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"conference"`
	Venue struct {
		Name string `json:"name"`
	} `json:"venue"`
}

func GetNHLTeams() (map[string]NHLTeamJSON, error) {
	res, err := http.Get(fmt.Sprintf("%s/teams", nhlURL))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var teamsJSON NHLTeamsJSON
	err = json.Unmarshal(body, &teamsJSON)
	if err != nil {
		return nil, err
	}

	ret := make(map[string]NHLTeamJSON)
	for _, team := range teamsJSON.Teams {
		ret[team.Abbreviation] = team
	}
	return ret, nil
}

type NHLGameCSVRow struct {
	GamePK      int64   `csv:"game_pk"`
	Date        string  `csv:"date"`
	Venue       string  `csv:"venue"`
	IsOT        int     `csv:"ot"`
	IsShootout  int     `csv:"shootout"`
	Status      string  `csv:"status"`
	HomeTeam    string  `csv:"home_team"`
	HomeScore   int     `csv:"home_score"`
	HomeELOPre  float64 `csv:"home_elo_pre"`
	HomeELOPost float64 `csv:"home_elo_post"`
	AwayTeam    string  `csv:"away_team"`
	AwayScore   int     `csv:"away_score"`
	AwayELOPre  float64 `csv:"away_elo_pre"`
	AwayELOPost float64 `csv:"away_elo_pre"`
}

func UpdateNHLSeason() error {
	season, err := GetNhlSeason()
	if err != nil {
		return err
	}

	teams, err := GetNHLTeams()
	if err != nil {
		return err
	}

	teamsByID := make(map[int]NHLTeamJSON)
	for _, team := range teams {
		teamsByID[team.ID] = team
	}

	elos, err := LoadPreseasonElos()
	if err != nil {
		return err
	}
	fmt.Printf("loaded %d elos\n", len(elos))

	gameRows := []NHLGameCSVRow{}
	for _, date := range season.Dates {
		for _, game := range date.Games {
			// skip preseason games
			if game.GameType == "PR" {
				continue
			}
			if game.GameType == "P" {
				// TODO: figure out if I want to handle playoffs
				continue
			}
			var isOT, isShootout int
			if game.Linescore.CurrentPeriodOrdinal == "OT" {
				isOT = 1
			} else if game.Linescore.CurrentPeriodOrdinal == "SO" {
				isOT = 1
				isShootout = 1
			}

			homeTeam := teamsByID[game.Teams.Home.Team.ID].Abbreviation
			awayTeam := teamsByID[game.Teams.Away.Team.ID].Abbreviation
			gameRow := NHLGameCSVRow{
				GamePK:     game.GamePK,
				Date:       date.Date,
				Status:     game.Status.AbstractGameState,
				Venue:      game.Venue.Name,
				HomeTeam:   homeTeam,
				HomeScore:  game.Teams.Home.Score,
				AwayTeam:   awayTeam,
				AwayScore:  game.Teams.Away.Score,
				IsOT:       isOT,
				IsShootout: isShootout,
			}

			if game.Status.AbstractGameState == "Final" {
				homeELOPre := elos[homeTeam]
				awayELOPre := elos[awayTeam]

				eloDiff := homeELOPre - awayELOPre

				if game.Venue.Name == teams[homeTeam].Venue.Name {
					// 50 points for home ice advantage
					eloDiff += 50
				}

				homeWinPct := 1.0 / (math.Pow(10, -eloDiff/400.0) + 1)
				shift := CalculateEloShift(eloDiff, homeWinPct, &gameRow)

				gameRow.HomeELOPre = homeELOPre
				gameRow.AwayELOPre = awayELOPre

				var homeELO, awayELO float64
				if gameRow.HomeScore > gameRow.AwayScore {
					homeELO = homeELOPre + shift
					awayELO = awayELOPre - shift
				} else {
					homeELO = homeELOPre - shift
					awayELO = awayELOPre + shift
				}
				gameRow.HomeELOPost = homeELO
				gameRow.AwayELOPost = awayELO
				elos[homeTeam] = homeELO
				elos[awayTeam] = awayELO
			}

			gameRows = append(gameRows, gameRow)
		}
	}

	eloFile, err := os.OpenFile(fmt.Sprintf("data/%s.csv", currentSeason), os.O_WRONLY|os.O_CREATE, os.ModePerm)
	if err != nil {
		return err
	}

	err = gocsv.MarshalFile(&gameRows, eloFile)
	return err
}

func LoadNHLSeason() ([]NHLGameCSVRow, error) {
	seasonFile, err := os.OpenFile(fmt.Sprintf("data/%s.csv", currentSeason), os.O_RDONLY, os.ModePerm)
	if err != nil {
		return nil, err
	}
	defer seasonFile.Close()

	season := []NHLGameCSVRow{}

	if err := gocsv.UnmarshalFile(seasonFile, &season); err != nil {
		return nil, err
	}

	return season, nil
}
