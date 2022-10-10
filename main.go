package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
)

func main() {
	// subcommands
	genPreseasonElo := flag.NewFlagSet("gen-preseason-elo", flag.ExitOnError)
	updateSeason := flag.NewFlagSet("update-season", flag.ExitOnError)
	simulate := flag.NewFlagSet("simulate", flag.ExitOnError)

	if len(os.Args) < 2 {
		fmt.Println("gen-preseason-elo or update-season command is required")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "gen-preseason-elo":
		genPreseasonElo.Parse(os.Args[2:])
	case "update-season":
		updateSeason.Parse(os.Args[2:])
	case "simulate":
		simulate.Parse(os.Args[2:])
	default:
		flag.PrintDefaults()
		os.Exit(1)
	}

	if genPreseasonElo.Parsed() {
		doGenPreseasonElo()
	} else if updateSeason.Parsed() {
		doUpdateSeason()
	} else if simulate.Parsed() {
		doSimulation()
	}
}

func doGenPreseasonElo() {
	elos, err := LoadLatestElo()
	if err != nil {
		fmt.Printf("could not load elo file: %s", err)
		os.Exit(1)
	}
	sort.Slice(elos, func(i, j int) bool {
		return elos[i].Date > elos[j].Date
	})

	currentElos := make(map[string]float64)
	for _, elo := range elos {
		if _, ok := currentElos[elo.HomeTeamAbbr]; !ok {
			currentElos[elo.HomeTeamAbbr] = (elo.HomeTeamPostgameRating * 0.7) + (1505 * 0.3)
		}
		if _, ok := currentElos[elo.AwayTeamAbbr]; !ok {
			currentElos[elo.AwayTeamAbbr] = (elo.AwayTeamPostgameRating * 0.7) + (1505 * 0.3)
		}
	}

	// wtf Nate
	currentElos["VGK"] = currentElos["VEG"]
	delete(currentElos, "VEG")

	WritePreseasonElos(currentElos)
}

func doUpdateSeason() {
	if err := UpdateNHLSeason(); err != nil {
		fmt.Printf("could not update season: %s", err)
		os.Exit(1)
	}
}

func doSimulation() {
	if err := RunSimulation(); err != nil {
		fmt.Printf("could not run simulation: %s", err)
		os.Exit(1)
	}
}
