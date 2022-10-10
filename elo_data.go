package main

import (
	"os"

	"github.com/gocarina/gocsv"
)

type GameEloDataRow struct {
	Season                 string  `csv:"season"`
	Date                   string  `csv:"date"`
	Playoff                int     `csv:"playoff"`
	Neutral                int     `csv:"neutral"`
	Status                 string  `csv:"status"`
	OT                     string  `csv:"ot"`
	HomeTeam               string  `csv:"home_team"`
	AwayTeam               string  `csv:"away_team"`
	HomeTeamAbbr           string  `csv:"home_team_abbr"`
	AwayTeamAbbr           string  `csv:"away_team_abbr"`
	HomeTeamPregameRating  float64 `csv:"home_team_pregame_rating"`
	AwayTeamPregameRating  float64 `csv:"away_team_pregame_rating"`
	HomeTeamWinprob        float64 `csv:"home_team_winprob"`
	AwayTeamWinprob        float64 `csv:"away_team_winprob"`
	OvertimeProb           float64 `csv:"overtime_prob"`
	HomeTeamExpectedPoints float64 `csv:"home_team_expected_points"`
	AwayTeamExpectedPoints float64 `csv:"away_team_expected_points"`
	HomeTeamScore          int     `csv:"home_team_score"`
	AwayTeamScore          int     `csv:"away_team_score"`
	HomeTeamPostgameRating float64 `csv:"home_team_postgame_rating"`
	AwayTeamPostgameRating float64 `csv:"away_team_postgame_rating"`
	GameQualityRating      int     `csv:"game_quality_rating"`
	GameImportanceRating   int     `csv:"game_importance_rating"`
	GameOverallRating      int     `csv:"game_overall_rating"`
}

func LoadLatestElo() ([]GameEloDataRow, error) {
	eloFile, err := os.OpenFile("data/nhl_elo_latest.csv", os.O_RDONLY, os.ModePerm)
	if err != nil {
		return nil, err
	}
	defer eloFile.Close()

	elos := []GameEloDataRow{}

	if err := gocsv.UnmarshalFile(eloFile, &elos); err != nil {
		return nil, err
	}

	return elos, nil
}

type PreaseaonElo struct {
	TeamAbbr string  `csv:"team_abbr"`
	Elo      float64 `csv:"elo"`
}

func WritePreseasonElos(elos map[string]float64) error {
	eloFile, err := os.OpenFile("data/preseason_elo.csv", os.O_WRONLY|os.O_CREATE, os.ModePerm)
	if err != nil {
		return err
	}
	defer eloFile.Close()

	eloSlice := []PreaseaonElo{}
	for abbr, elo := range elos {
		eloSlice = append(eloSlice, PreaseaonElo{TeamAbbr: abbr, Elo: elo})
	}

	err = gocsv.MarshalFile(&eloSlice, eloFile)
	return err
}

func LoadPreseasonElos() (map[string]float64, error) {
	eloFile, err := os.OpenFile("data/preseason_elo.csv", os.O_RDONLY, os.ModePerm)
	if err != nil {
		return nil, err
	}
	defer eloFile.Close()

	eloSlice := []PreaseaonElo{}
	if err := gocsv.UnmarshalFile(eloFile, &eloSlice); err != nil {
		return nil, err
	}

	ret := make(map[string]float64)
	for _, elo := range eloSlice {
		ret[elo.TeamAbbr] = elo.Elo
	}
	return ret, nil
}
