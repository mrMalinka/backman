package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/lithammer/fuzzysearch/fuzzy"
)

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("	help => Show this menu")
	fmt.Println("	info => Print config info and exit")
	fmt.Println("	backup [dir] => Which directory to backup, defaults to `.`")
	fmt.Println("	restore [id] => Restores from a backup, use `list` to get ID's")
	fmt.Println("	list [query] => List backups with filter, omit query to list all")
	fmt.Println("	delete [id] => Delete a backup with given ID")
	fmt.Println("	purge [date] => Delete backups older than date. (go time format, eg '1d1h')")
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
			"time format":     config.TimeFormat,
		}
		for k, v := range info {
			fmt.Printf("%s: %v\n", k, v)
		}
		return
	case "backup":
		target := "."
		if len(os.Args) > 2 {
			target = os.Args[2]
		}

		makeBackup(target)
		return
	case "restore":
		if len(os.Args) < 3 {
			break
		}
		restoreFrom(readUint16Fatal(os.Args[2]))
		return
	case "list":
		if len(os.Args) > 2 {
			listBackups(os.Args[2])
		} else {
			listBackups("")
		}
		return
	case "delete":
		if len(os.Args) < 3 {
			break
		}
		deleteBackup(readUint16Fatal(os.Args[2]))
		return
	case "purge":
		if len(os.Args) < 3 {
			break
		}

		// parse the threshold
		age, err := parseDurationExt(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid duration %q: %v\n", os.Args[2], err)
			os.Exit(1)
		}
		cutoff := time.Now().Add(-age)
		purgeBackups(cutoff)
		return
	}

	printUsage()
	os.Exit(1)
}

func makeBackup(target string) {
	appDir := getAppDir()
	if _, err := os.Stat(appDir); errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Directory '%s' missing, creating...\n", appDir)
		err := os.MkdirAll(appDir, 0755)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error creating backup directory: ", err)
			os.Exit(1)
		}
	}

	uuid := generateUUID()

	targetAbs, err := filepath.Abs(target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error getting absolute path of target: ", err)
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
		fmt.Fprintln(os.Stderr, "error generating sidecar file: ", err)
		os.Exit(1)
	}

	fmt.Println("Compressing directory...")
	// compress directory and copy into backupName
	err = compressDir(target, backupName)

	if err != nil {
		// undo if compression failed
		fmt.Fprintln(os.Stderr, "error compressing directory: ", err)
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
		fmt.Fprintln(os.Stderr, "error reading sidecar files: ", err)
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
		fmt.Fprintln(os.Stderr, "error decompressing directory: ", err)
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

	sort.Slice(sidecars, func(i, j int) bool {
		return sidecars[i].Time.Before(sidecars[j].Time)
	})

	var q string
	if query != "" {
		q = strings.ToLower(query)
	}

	for _, data := range sidecars {
		var prefix, suffix string
		if query == "" {
			// normal text
			prefix = ""
			suffix = ""
		} else {
			hay := data.FormatHay()
			if fuzzy.Match(q, hay) {
				// matching bold
				prefix = "\033[1m"
				suffix = "\033[0m"
			} else {
				// non matching grey
				prefix = "\033[90m"
				suffix = "\033[0m"
			}
		}

		fmt.Printf("%s%v:\n\t%s\n\t%s | %s\n%s",
			prefix,
			data.ID,
			data.BackupOf,
			data.Time.Local().Format(config.TimeFormat),
			humanize.IBytes(uint64(data.ParentSize)),
			suffix,
		)
	}
}
func deleteBackup(id uint16) {
	files, err := readSidecars()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error reading sidecar files: ", err)
		os.Exit(1)
	}

	for _, file := range files {
		if file.ID == id {
			file.DeleteAll()
			fmt.Println("Deleted successfully")
			return
		}
	}

	fmt.Fprintln(os.Stderr, "ID not found!")
	os.Exit(1)
}
func purgeBackups(cutoff time.Time) {
	sidecars, err := readSidecars()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading sidecar files: %v\n", err)
		os.Exit(1)
	}

	// delete any older than cutoff
	var deleted int
	for _, sc := range sidecars {
		if sc.Time.Before(cutoff) {
			sc.DeleteAll()
			deleted++
		}
	}

	fmt.Printf("Purged %d backups!\n", deleted)
}
