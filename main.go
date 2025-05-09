package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/lithammer/fuzzysearch/fuzzy"
)

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("	help => Show this menu")
	fmt.Println("	backup [dir] => Which directory to backup, defaults to `.`")
	fmt.Println("	restore [id] => Restores from a backup, use `list` to get ID's")
	fmt.Println("	list => List backups")
	fmt.Println("	info => Print config info and exit")
}

func main() {
	loadConfig()

	if len(os.Args) < 2 || os.Args[1] == "help" {
		printUsage()
		return
	}

	switch os.Args[1] {
	case "info":
		info := map[string]any{
			"backup location": config.ArchiveDir,
		}
		for k, v := range info {
			fmt.Printf("%s: %v\n", k, v)
		}
	case "backup":
		target := "."
		if len(os.Args) > 2 {
			target = os.Args[2]
		}

		makeBackup(target)
	case "restore":
		if len(os.Args) < 3 {
			break
		}
		rawID, err := strconv.Atoi(os.Args[2])
		if err != nil {
			fmt.Println("error parsing ID: ", err)
			os.Exit(1)
		}
		if rawID < 0 || rawID > math.MaxUint16 {
			fmt.Println("Please enter a valid ID. (0-65535)")
			os.Exit(1)
		}

		id := uint16(rawID)
		restoreFrom(id)
	case "list":
		if len(os.Args) > 2 {
			listBackups(os.Args[2])
		} else {
			listBackups("")
		}
	default:
		// print usage if no arguments
		printUsage()
		os.Exit(1)
	}
}

func makeBackup(target string) {
	appDir := getAppDir()
	if _, err := os.Stat(appDir); errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Directory '%s' missing, creating...\n", appDir)
		err := os.MkdirAll(appDir, 0755)
		if err != nil {
			fmt.Println("error creating backup directory: ", err)
			os.Exit(1)
		}
	}

	uuid := generateUUID()

	targetAbs, err := filepath.Abs(target)
	if err != nil {
		fmt.Println("error getting absolute path of target: ", err)
		os.Exit(1)
	}

	backupName := filepath.Join(
		config.ArchiveDir, fmt.Sprintf("%s.tar.zstd", uuid),
	)
	sidecarName := backupName + ".json"

	fmt.Println("Generating sidecar file...")

	// generate sidecar file
	deleteSidecar, err := generateSidecar(sidecarName, targetAbs)
	if err != nil {
		fmt.Println("error generating sidecar file: ", err)
		os.Exit(1)
	}

	fmt.Println("Compressing directory...")
	// compress directory and copy into backupName
	err = compressDir(target, backupName)

	if err != nil {
		// undo if compression failed
		fmt.Println("error compressing directory: ", err)
		deleteSidecar()
		os.Exit(1)
	}

	fmt.Printf(
		"\nDone.\n Original size: %s\n Compressed size: %s\n",
		humanize.IBytes(uint64(dirSize(target))),
		humanize.IBytes(uint64(fileSize(backupName))),
	)
}
func restoreFrom(id uint16) {
	sidecars, err := readSidecars()
	if err != nil {
		fmt.Println("error reading sidecar files: ", err)
		os.Exit(1)
	}

	var backupSidecar SidecarData
	for _, sidecar := range sidecars {
		if sidecar.ID == id {
			backupSidecar = sidecar
			break
		}
	}

	// ./some_directory-restored
	restoringTo := filepath.Base(backupSidecar.BackupOf) + "-restored"

	err = decompressDir(backupSidecar.ParentPath, restoringTo)
	if err != nil {
		fmt.Println("error decompressing directory: ", err)
		os.Exit(1)
	}

	fmt.Printf("Restored backup into '%s'\n", restoringTo)
}
func listBackups(query string) {
	sidecars, err := readSidecars()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading sidecar files: %v\n", err)
		os.Exit(1)
	}

	// sort by which was created earliest
	sort.Slice(sidecars, func(i, j int) bool {
		return sidecars[i].Time.Before(sidecars[j].Time)
	})

	var filtered []SidecarData
	if query != "" {
		q := strings.ToLower(query)
		for _, sc := range sidecars {
			hay := sc.FormatHay()
			if fuzzy.Match(q, hay) {
				filtered = append(filtered, sc)
			}
		}
	} else {
		filtered = sidecars
	}

	for _, data := range filtered {
		fmt.Printf(
			"%v:\n\t%s\n\t%s | %s\n",
			data.ID,
			data.BackupOf,
			data.Time.Local().Format(config.TimeFormat),
			humanize.IBytes(uint64(data.ParentSize)),
		)
	}
}
