package main

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

type SidecarData struct {
	// the absolute path of the directory that the parent file of this sidecar is backing up
	BackupOf string `json:"of"`
	// when the backup was created
	Time time.Time `json:"time"`
	// unique id
	ID uint16 `json:"id"`

	// populated by readSidecars
	ParentSize int64
	ParentPath string
}

func (s *SidecarData) FormatHay() string {
	return strings.ToLower(fmt.Sprintf(
		"%v %s %s",
		s.ID, s.BackupOf,
		s.Time.Local().Format(config.TimeFormat),
	))
}
func (s *SidecarData) DeleteAll() {
	if s.ParentPath != "" {
		os.Remove(s.ParentPath)
		os.Remove(s.ParentPath + ".json")
	}
}

// important: name and backupOf should be absolute paths
func generateSidecar(name, backupOf string) (func(), error) {
	// read other sidecars to check which ID's have already been used
	var usedIDs []uint16
	others, err := readSidecars()
	if err != nil {
		return nil, fmt.Errorf("error reading other sidecars: %w", err)
	}
	for _, sidecar := range others {
		usedIDs = append(usedIDs, sidecar.ID)
	}

	sidecarData := SidecarData{
		BackupOf: backupOf,
		Time:     time.Now().Local(),
		ID:       closestMissing(usedIDs),
	}

	data, err := json.Marshal(sidecarData)
	if err != nil {
		return nil, err
	}

	return func() {
		os.Remove(name)
	}, os.WriteFile(name, data, 0600)
}

func readSidecars() ([]SidecarData, error) {
	appDir := getAppDir()

	if _, err := os.Stat(appDir); errors.Is(err, os.ErrNotExist) {
		// return empty list if the directory wasnt found
		return []SidecarData{}, nil
	}

	entries, err := os.ReadDir(appDir)
	if err != nil {
		return nil, err
	}

	var dataEntries []SidecarData

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" || entry.IsDir() {
			continue
		}

		var sidecarData SidecarData
		entryAbs := filepath.Join(appDir, entry.Name())
		parentAbs := strings.TrimSuffix(entryAbs, ".json")

		_, err := os.Stat(parentAbs)
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(os.Stderr, "WARNING: Sidecar without parent found. Deleting...")
			os.Remove(entryAbs)
			continue
		}

		data, err := os.ReadFile(entryAbs)
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal(data, &sidecarData)
		if err != nil {
			fmt.Printf("WARNING: Error parsing sidecar file. (%s)\n", entry.Name())
			if askYesNo("Delete?") {
				// remove sidecar
				os.Remove(entryAbs)
				// remove backup file aswell
				os.Remove(strings.TrimSuffix(entryAbs, ".json"))
			}
			continue
		}

		sidecarData.ParentSize = fileSize(parentAbs)
		sidecarData.ParentPath = parentAbs

		dataEntries = append(dataEntries, sidecarData)
	}

	return dataEntries, nil
}

func compressDir(src, dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()

	enc, err := zstd.NewWriter(f, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if err != nil {
		return err
	}
	defer enc.Close()

	tarWriter := tar.NewWriter(enc)
	defer tarWriter.Close()

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == src {
			return nil
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, relPath)
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tarWriter, file)
		return err
	})
}
func decompressDir(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	dec, err := zstd.NewReader(f)
	if err != nil {
		return err
	}
	defer dec.Close()

	tr := tar.NewReader(dec)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		targetPath := filepath.Join(dst, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return err
			}

		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()

		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, targetPath); err != nil {
				return err
			}

		default:
			continue
		}
	}

	return nil
}
