package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bogem/id3v2/v2"
	"github.com/dmulholl/mp3lib"
)

func runMerge(args []string) error {
	fs := flag.NewFlagSet("merge", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: soundrig merge <directory>\n\n")
		fmt.Fprintf(os.Stderr, "Merge all MP3 files in <directory> into a single output file.\n")
		fmt.Fprintf(os.Stderr, "Output filename is derived from the directory name.\n")
	}
	fs.Parse(args)

	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}

	inputDir := strings.TrimRight(fs.Arg(0), "/")
	outputFile := strings.ToLower(inputDir) + ".mp3"

	if err := mergeMp3Files(inputDir, outputFile); err != nil {
		return err
	}

	return addTags(outputFile)
}

func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
		}
	}
	return strings.Join(words, " ")
}

func parseFilename(filename string) (date, author, albumTitle string) {
	pattern := regexp.MustCompile(`(\d{4}-\d{2}-\d{2})_(.+)_(.+)\.mp3`)
	match := pattern.FindStringSubmatch(filename)
	if match == nil {
		return "", "", ""
	}
	date = match[1]
	author = titleCase(strings.ReplaceAll(match[2], "-", " "))
	title := titleCase(strings.ReplaceAll(match[3], "-", " "))
	albumTitle = fmt.Sprintf("%s - %s - %s", date, author, title)
	return date, author, albumTitle
}

func addTags(filePath string) error {
	tag, err := id3v2.Open(filePath, id3v2.Options{Parse: true})
	if err != nil {
		return err
	}
	defer tag.Close()

	filename := filepath.Base(filePath)
	date, author, albumTitle := parseFilename(filename)
	if author == "" || albumTitle == "" {
		return nil
	}

	tag.SetArtist(author)
	tag.SetAlbum(albumTitle)
	tag.SetTitle(albumTitle)
	tag.SetYear(date)

	return tag.Save()
}

func mergeMp3Files(inputDir, outputFile string) error {
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return err
	}

	var mp3Files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".mp3") {
			mp3Files = append(mp3Files, e.Name())
		}
	}

	if len(mp3Files) == 0 {
		fmt.Println("No MP3 files found in the specified directory.")
		return nil
	}

	sort.Strings(mp3Files)

	outFile, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer outFile.Close()

	var totalFrames uint32
	var totalBytes uint32

	for _, f := range mp3Files {
		fmt.Printf("processing: %s\n", f)
		filePath := filepath.Join(inputDir, f)

		inFile, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("opening %s: %w", f, err)
		}

		for {
			obj := mp3lib.NextObject(inFile)
			if obj == nil {
				break
			}

			frame, ok := obj.(*mp3lib.MP3Frame)
			if !ok {
				continue
			}

			if mp3lib.IsXingHeader(frame) || mp3lib.IsVbriHeader(frame) {
				continue
			}

			_, err := outFile.Write(frame.RawBytes)
			if err != nil {
				inFile.Close()
				return fmt.Errorf("writing frame: %w", err)
			}

			totalFrames++
			totalBytes += uint32(len(frame.RawBytes))
		}

		inFile.Close()
	}

	if totalFrames > 0 {
		xingHeader := mp3lib.NewXingHeader(totalFrames, totalBytes)

		tmpFile := outputFile + ".tmp"
		tmp, err := os.Create(tmpFile)
		if err != nil {
			return fmt.Errorf("creating temp file: %w", err)
		}

		if _, err := tmp.Write(xingHeader.RawBytes); err != nil {
			tmp.Close()
			os.Remove(tmpFile)
			return fmt.Errorf("writing xing header: %w", err)
		}

		outFile.Close()
		src, err := os.Open(outputFile)
		if err != nil {
			tmp.Close()
			os.Remove(tmpFile)
			return err
		}

		buf := make([]byte, 32*1024)
		for {
			n, err := src.Read(buf)
			if n > 0 {
				if _, werr := tmp.Write(buf[:n]); werr != nil {
					src.Close()
					tmp.Close()
					os.Remove(tmpFile)
					return werr
				}
			}
			if err != nil {
				break
			}
		}

		src.Close()
		tmp.Close()

		if err := os.Rename(tmpFile, outputFile); err != nil {
			return fmt.Errorf("renaming temp file: %w", err)
		}
	}

	fmt.Printf("Merged %d MP3 files into %s\n", len(mp3Files), outputFile)
	return nil
}
