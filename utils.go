package main

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func askYesNo(prompt string) bool {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s [y/n]: ", prompt)

		input, err := reader.ReadString('\n')
		if err != nil {
			return false
		}

		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "y", "yes":
			return true
		default:
			return false
		}
	}
}

func readUint16Fatal(str string) uint16 {
	rawID, err := strconv.Atoi(str)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error parsing ID: ", err)
		os.Exit(1)
	}
	if rawID < 0 || rawID > math.MaxUint16 {
		fmt.Fprintln(os.Stderr, "Please enter a valid ID. (0-65535)")
		os.Exit(1)
	}

	return uint16(rawID)
}

func closestMissing(nums []uint16) uint16 {
	n := len(nums)
	if n == 0 {
		return 0
	}

	sort.Slice(nums, func(i, j int) bool { return nums[i] < nums[j] })
	write := 1
	for read := 1; read < n; read++ {
		if nums[read] != nums[write-1] {
			nums[write] = nums[read]
			write++
		}
	}
	nums = nums[:write]

	bestGap := 0
	bestIdx := 0
	for i := 0; i < len(nums)-1; i++ {
		gap := int(nums[i+1]) - int(nums[i]) - 1
		if gap > bestGap {
			bestGap = gap
			bestIdx = i
		}
	}

	if bestGap > 0 {
		// Instead of picking the midpoint, pick the first missing value:
		return nums[bestIdx] + 1
	}

	maxv := nums[len(nums)-1]
	if maxv < math.MaxUint16 {
		return maxv + 1
	}

	minv := nums[0]
	if minv > 0 {
		return minv - 1
	}

	return 0
}

func generateUUID() string {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		// realistically this is never gonna happen anyways
		fmt.Fprintln(os.Stderr, "error reading from rand: ", err)
		os.Exit(1)
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	uuid := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16],
	)
	return uuid
}

func parseDurationExt(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func dirSize(root string) int64 {
	var totalSize int64

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.Mode().IsRegular() {
			totalSize += info.Size()
		}
		return nil
	})
	if err != nil {
		return -1
	}

	return totalSize
}
func fileSize(file string) int64 {
	info, err := os.Stat(file)
	if err != nil {
		return -1
	}

	return info.Size()
}
