package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	"gonum.org/v1/gonum/stat/distuv"
)

type TeamSimulationResults struct {
	MadePlayoffs int
	D1Seed       int
	D2Seed       int
	D3Seed       int
	WC1          int
	WC2          int
}

const numRuns = 1000000

func RunSimulation() error {
	// 1665171464 generates 3-way tie
	seed := time.Now().Unix()
	//seed := int64(1665171464)
	rand.Seed(seed)
	fmt.Printf("using seed %d\n", seed)
	elos, err := LoadPreseasonElos()
	if err != nil {
		return err
	}
	fmt.Printf("loaded %d elos\n", len(elos))

	season, err := LoadNHLSeason()
	if err != nil {
		return err
	}
	fmt.Printf("loaded %d games\n", len(season))

	teams, err := GetNHLTeams()
	if err != nil {
		return err
	}
	fmt.Printf("loaded %d teams\n", len(teams))

	// one pass to grab any updated elos
	for _, game := range season {
		if game.AwayELOPost > 0 {
			fmt.Printf("Updating ELO for %s from game %d\n", game.AwayTeam, game.GamePK)
			elos[game.AwayTeam] = game.AwayELOPost
		}
		if game.HomeELOPost > 0 {
			fmt.Printf("Updating ELO for %s from game %d\n", game.AwayTeam, game.GamePK)
			elos[game.HomeTeam] = game.HomeELOPost
		}
	}

	simulationResults := make(map[string]*TeamSimulationResults)
	for _, team := range teams {
		simulationResults[team.Abbreviation] = &TeamSimulationResults{}
	}

	start := time.Now()

	for i := 0; i < numRuns; i++ {
		simulatedSeason := SimulateSeason(&elos, &season, &teams)
		simulatedStandings := CalculateStandings(&teams, &simulatedSeason)
		for _, divisionStandings := range simulatedStandings.DivisionSeeds {
			for i, team := range divisionStandings {
				teamStandings := simulationResults[team]
				teamStandings.MadePlayoffs += 1
				if i == 0 {
					teamStandings.D1Seed += 1
				} else if i == 1 {
					teamStandings.D2Seed += 1
				} else if i == 2 {
					teamStandings.D3Seed += 1
				}
			}
		}
		for _, conferenceWildCards := range simulatedStandings.WildCards {
			for i, team := range conferenceWildCards {
				teamStandings := simulationResults[team]
				teamStandings.MadePlayoffs += 1
				if i == 0 {
					teamStandings.WC1 += 1
				} else if i == 1 {
					teamStandings.WC2 += 1
				}
			}
		}
	}

	duration := time.Since(start)
	fmt.Printf("execution took %s\n", duration)

	fmt.Print("results:\n")
	for team, standings := range simulationResults {
		playoffChance := float64(standings.MadePlayoffs) / float64(numRuns)
		d1Chance := 100. * float64(standings.D1Seed) / float64(numRuns)
		d2Chance := 100. * float64(standings.D2Seed) / float64(numRuns)
		d3Chance := 100. * float64(standings.D3Seed) / float64(numRuns)
		wc1Chance := 100. * float64(standings.WC1) / float64(numRuns)
		wc2Chance := 100. * float64(standings.WC2) / float64(numRuns)
		fmt.Printf("%s: %f%% playoffs (%f D1, %f D2, %f D3, %f WC1, %f WC2)\n", team, playoffChance, d1Chance, d2Chance, d3Chance, wc1Chance, wc2Chance)
	}

	return nil
}

func SimulateSeason(baseElos *map[string]float64, baseSeason *[]NHLGameCSVRow, teams *map[string]NHLTeamJSON) []NHLGameCSVRow {
	// copy the elo map so we can keep it updated for this simulation
	seasonElos := make(map[string]float64)
	for team, elo := range *baseElos {
		seasonElos[team] = elo
	}

	seasonGames := []NHLGameCSVRow{}
	for _, game := range *baseSeason {
		if game.Status == "Final" {
			seasonGames = append(seasonGames, game)
			continue
		}

		seasonGames = append(seasonGames, SimulateGame(game, seasonElos, teams))
	}

	return seasonGames
}

func SimulateGame(game NHLGameCSVRow, elos map[string]float64, teams *map[string]NHLTeamJSON) NHLGameCSVRow {
	simulatedGame := game
	simulatedGame.Status = "Simulated"

	homeElo := elos[game.HomeTeam]
	if (*teams)[game.HomeTeam].Venue.Name == game.Venue {
		// 50 points for home ice advantage
		homeElo += 50
	}
	awayElo := elos[game.AwayTeam]
	eloDiff := homeElo - awayElo
	/*if game.IsPlayoff == 1 {
		eloDiff = eloDiff * 1.25
	}*/
	homeWinPct := 1.0 / (math.Pow(10, -eloDiff/400.0) + 1)

	//fmt.Printf("%s (elo %f) vs. %s (elo %f): %f\n", game.HomeTeam, homeElo-50, game.AwayTeam, awayElo, homeWinPct)

	isHomeWin := rand.Float64() < homeWinPct
	//fmt.Printf("  simulated home win? %t\n", isHomeWin)

	otChance := 1.0 / (1 + math.Exp(-1.0*(-1.1320032+(-0.0009822*eloDiff))))
	//fmt.Printf("  ot chance: %f\n", otChance)
	isOT := rand.Float64() < otChance
	isShootout := isOT && rand.Intn(2) == 0
	//fmt.Printf("  is OT: %t, is shootout %t\n", isOT, isShootout)

	homePoisson := distuv.Poisson{Lambda: 2.8411351 + (0.0042408 * eloDiff)}
	awayPoisson := distuv.Poisson{Lambda: 2.8411351 + (0.0042408 * -eloDiff)}
	var homeScore, awayScore, goalDiff int
	attempts := 0
	for {
		attempts += 1
		homeScore = int(homePoisson.Rand())
		awayScore = int(awayPoisson.Rand())
		if homeScore > awayScore {
			goalDiff = homeScore - awayScore
		} else {
			goalDiff = awayScore - homeScore
		}

		// make sure the right team won
		if (isHomeWin && homeScore > awayScore) || (!isHomeWin && awayScore > homeScore) {
			// make sure if OT the score is within 1 point
			if !isOT || (isOT && goalDiff == 1) {
				break
			}
		}
	}
	//fmt.Printf("  simulated score: %d - %d (after %d attempts) \n", homeScore, awayScore, attempts)

	simulatedGame.HomeScore = homeScore
	simulatedGame.AwayScore = awayScore
	if isOT {
		simulatedGame.IsOT = 1
	}
	if isShootout {
		simulatedGame.IsShootout = 1
	}

	shift := CalculateEloShift(eloDiff, homeWinPct, &simulatedGame)

	//fmt.Printf("  shift: %f (mov: %f, aca: %f, pgf: %f)\n", shift, marginOfVictoryMultiplier, autocorrelationAdjustment, pregameFavoriteMultiplier)

	if isHomeWin {
		elos[game.HomeTeam] += shift
		elos[game.AwayTeam] -= shift
	} else {
		elos[game.AwayTeam] += shift
		elos[game.HomeTeam] -= shift
	}

	//fmt.Printf("  new elos: %s: %f, %s: %f\n", game.HomeTeam, elos[game.HomeTeam], game.AwayTeam, elos[game.AwayTeam])

	return simulatedGame
}

func CalculateEloShift(eloDiff float64, homeWinPct float64, game *NHLGameCSVRow) float64 {
	var winnerEloDiff float64
	var goalDiff int
	if game.HomeScore > game.AwayScore {
		winnerEloDiff = eloDiff
		goalDiff = game.HomeScore - game.AwayScore
	} else {
		winnerEloDiff = -eloDiff
		goalDiff = game.AwayScore - game.HomeScore
	}

	marginOfVictoryMultiplier := (0.6686 * math.Log(float64(goalDiff))) + 0.8048
	autocorrelationAdjustment := 2.05 / ((winnerEloDiff * 0.001) + 2.05)

	var teamWin float64
	var teamWinProb float64
	if homeWinPct < 0.5 {
		// away favorite
		if game.HomeScore < game.AwayScore {
			teamWin = 1.0
		}
		teamWinProb = 1.0 - homeWinPct
	} else {
		// home favorite
		if game.HomeScore > game.AwayScore {
			teamWin = 1.0
		}
		teamWinProb = homeWinPct
	}
	pregameFavoriteMultiplier := teamWin - teamWinProb

	return 6.0 * marginOfVictoryMultiplier * autocorrelationAdjustment * pregameFavoriteMultiplier
}

type NHLSeasonStats struct {
	Team           string
	Wins           int
	Losses         int
	RegulationWins int
	OTWins         int
	SOWins         int
	Points         int
	GoalsFor       int
	GoalsAgainst   int
}

type GamesWonTiebreakerKey struct {
	Points int
	RW     int
	OT     int
	SO     int
}

type Standings struct {
	DivisionSeeds map[string][]string
	WildCards     map[string][]string
}

func CalculateStandings(teams *map[string]NHLTeamJSON, games *[]NHLGameCSVRow) Standings {
	seasonStats := make(map[string]*NHLSeasonStats)
	for abbr := range *teams {
		seasonStats[abbr] = &NHLSeasonStats{Team: abbr}
	}

	for _, game := range *games {
		homeTeamStats := seasonStats[game.HomeTeam]
		awayTeamStats := seasonStats[game.AwayTeam]

		var winner, loser *NHLSeasonStats
		if game.HomeScore > game.AwayScore {
			winner = homeTeamStats
			loser = awayTeamStats
		} else {
			winner = awayTeamStats
			loser = homeTeamStats
		}

		winner.Wins += 1
		winner.Points += 2
		loser.Losses += 1
		if game.IsShootout == 1 {
			winner.SOWins += 1
			loser.Points += 1
		} else if game.IsOT == 1 {
			winner.OTWins += 1
			loser.Points += 1
		} else {
			winner.RegulationWins += 1
		}

		homeTeamStats.GoalsFor += game.HomeScore
		homeTeamStats.GoalsAgainst += game.AwayScore
		awayTeamStats.GoalsFor += game.AwayScore
		awayTeamStats.GoalsAgainst += game.HomeScore
	}

	h2hTiebreakers := make(map[GamesWonTiebreakerKey][]string)

	finalSeasonStats := []*NHLSeasonStats{}
	for _, stats := range seasonStats {
		finalSeasonStats = append(finalSeasonStats, stats)

		key := GamesWonTiebreakerKey{
			Points: stats.Points,
			RW:     stats.RegulationWins,
			OT:     stats.OTWins,
			SO:     stats.SOWins,
		}
		h2hTiebreakers[key] = append(h2hTiebreakers[key], stats.Team)
	}

	// figure out which teams we need to sort out the points earned in relevant games tiebreaker
	h2hTiebreakerRanks := make(map[string]int)
	for _, teams := range h2hTiebreakers {
		if len(teams) > 1 {
			for team, rank := range GamesPlayedTiebreak(teams, games) {
				h2hTiebreakerRanks[team] = rank
			}
		}
	}

	sort.Slice(finalSeasonStats, func(i, j int) bool {
		statsI := finalSeasonStats[i]
		statsJ := finalSeasonStats[j]
		if statsI.Points != statsJ.Points {
			return statsI.Points > statsJ.Points
		}
		if statsI.RegulationWins != statsJ.RegulationWins {
			return statsI.RegulationWins > statsJ.RegulationWins
		}
		if statsI.OTWins != statsJ.OTWins {
			return statsI.OTWins > statsJ.OTWins
		}
		if statsI.SOWins != statsJ.SOWins {
			return statsI.SOWins > statsJ.SOWins
		}
		// h2h tiebreaker, should have an entry
		if _, ok := h2hTiebreakerRanks[statsI.Team]; !ok {
			panic(fmt.Sprintf("team %s should be in h2h tiebreaker map: %+v\n\n%+v", statsI.Team, h2hTiebreakers, h2hTiebreakerRanks))
		}
		if _, ok := h2hTiebreakerRanks[statsJ.Team]; !ok {
			panic(fmt.Sprintf("team %s should be in h2h tiebreaker map: %+v\n\n%+v", statsJ.Team, h2hTiebreakers, h2hTiebreakerRanks))
		}
		if h2hTiebreakerRanks[statsI.Team] != h2hTiebreakerRanks[statsJ.Team] {
			return h2hTiebreakerRanks[statsI.Team] > h2hTiebreakerRanks[statsJ.Team]
		}
		statsIGDiff := statsI.GoalsFor - statsI.GoalsAgainst
		statsJGDiff := statsJ.GoalsFor - statsJ.GoalsAgainst
		if statsIGDiff != statsJGDiff {
			return statsIGDiff > statsJGDiff
		}
		if statsI.GoalsFor != statsJ.GoalsFor {
			return statsI.GoalsFor > statsJ.GoalsFor
		}
		fmt.Printf("cannot determine ordering between %+v and %+v", *statsI, *statsJ)
		return true
	})

	divisionSeeds := make(map[string][]string)
	conferenceWildCards := make(map[string][]string)

	for _, teamStat := range finalSeasonStats {
		team := (*teams)[teamStat.Team]
		ds := divisionSeeds[team.Division.Name]
		wc := conferenceWildCards[team.Conference.Name]
		if len(ds) < 3 {
			divisionSeeds[team.Division.Name] = append(divisionSeeds[team.Division.Name], teamStat.Team)
		} else if len(wc) < 2 {
			conferenceWildCards[team.Conference.Name] = append(conferenceWildCards[team.Conference.Name], teamStat.Team)
		}
	}

	/*fmt.Print("stats:\n")
	for _, stats := range finalSeasonStats {
		fmt.Printf("  %s: (%d, %d, %d); %d points; %d GF; %d GA\n", stats.Team, stats.RegulationWins, stats.OTWins+stats.SOWins, stats.Losses, stats.Points, stats.GoalsFor, stats.GoalsAgainst)
	}
	fmt.Printf("seeds:\n%+v\n", divisionSeeds)
	fmt.Printf("wildcards:\n%+v\n", conferenceWildCards)*/

	return Standings{
		DivisionSeeds: divisionSeeds,
		WildCards:     conferenceWildCards,
	}

}

func GamesPlayedTiebreak(teams []string, games *[]NHLGameCSVRow) map[string]int {
	if len(teams) > 2 {
		fmt.Printf(" determining tiebreak for teams: %+v\n", teams)
	}
	teamSet := make(map[string]bool)
	for _, team := range teams {
		teamSet[team] = true
	}
	teamGames := make(map[string][]NHLGameCSVRow)
	gamesByHomeAway := make(map[string]int)

	for _, game := range *games {
		_, homePresent := teamSet[game.HomeTeam]
		_, awayPresent := teamSet[game.AwayTeam]
		if homePresent && awayPresent {
			teamGames[game.HomeTeam] = append(teamGames[game.HomeTeam], game)
			teamGames[game.AwayTeam] = append(teamGames[game.AwayTeam], game)
			gamesByHomeAway[fmt.Sprintf("%s%s", game.HomeTeam, game.AwayTeam)] += 1
		}
	}

	teamWinPcts := make(map[float64][]string)

	for team, consideredGames := range teamGames {
		var ptsWon, ptsAvailable int
		needSkip := len(consideredGames)%2 != 0

		if len(teams) > 2 {
			fmt.Printf(" considering games %+v for team %s\n", consideredGames, team)
		}

		for _, game := range consideredGames {
			if needSkip && gamesByHomeAway[fmt.Sprintf("%s%s", game.HomeTeam, game.AwayTeam)] > gamesByHomeAway[fmt.Sprintf("%s%s", game.AwayTeam, game.HomeTeam)] {
				if len(teams) > 2 {
					fmt.Printf(" skipping game: %+v\n", game)
				}
				needSkip = false
				continue
			}
			isWin := (team == game.HomeTeam && game.HomeScore > game.AwayScore) || (team == game.AwayTeam && game.AwayScore > game.HomeScore)

			ptsAvailable += 2
			if isWin {
				ptsWon += 2
			}
			if game.IsOT == 1 {
				ptsAvailable += 1
			}
		}

		pctWon := float64(ptsWon) / float64(ptsAvailable)
		if len(teams) > 2 {
			fmt.Printf(" team %s won %d out of %d points (%f%%)\n", team, ptsWon, ptsAvailable, pctWon*100)
		}

		teamWinPcts[pctWon] = append(teamWinPcts[pctWon], team)
	}

	pcts := []float64{}
	for pct := range teamWinPcts {
		pcts = append(pcts, pct)
	}
	sort.Float64s(pcts)

	teamRanks := make(map[string]int)
	for i, pct := range pcts {
		for _, team := range teamWinPcts[pct] {
			teamRanks[team] = i
		}
	}
	if len(teams) > 2 {
		fmt.Printf(" final team ranks: %+v\n", teamRanks)
	}

	return teamRanks
}
