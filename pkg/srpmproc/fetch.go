package srpmproc

import (
	"errors"
	"fmt"
	"github.com/rocky-linux/srpmproc/pkg/blob"
	"github.com/rocky-linux/srpmproc/pkg/data"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func Fetch(logger io.Writer, cdnUrl string, dir string, storage blob.Storage) error {
	pd := &data.ProcessData{
		Log: log.New(logger, "", log.LstdFlags),
	}

	metadataPath := ""
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if strings.HasSuffix(path, ".metadata") {
			if metadataPath != "" {
				return errors.New("multiple metadata files")
			}
			metadataPath = path
		}
		return nil
	})
	if err != nil {
		return err
	}

	metadataFile, err := os.Open(metadataPath)
	if err != nil {
		return fmt.Errorf("could not open metadata file: %v", err)
	}

	fileBytes, err := ioutil.ReadAll(metadataFile)
	if err != nil {
		return fmt.Errorf("could not read metadata file: %v", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			DisableCompression: false,
		},
	}
	fileContent := strings.Split(string(fileBytes), "\n")
	for _, line := range fileContent {
		if strings.TrimSpace(line) == "" {
			continue
		}

		lineInfo := strings.Split(line, " ")
		hash := lineInfo[0]
		path := lineInfo[1]

		url := fmt.Sprintf("%s/%s", cdnUrl, hash)
		if storage != nil {
			url = hash
		}
		pd.Log.Printf("downloading %s", url)

		var body []byte

		if storage != nil {
			body, err = storage.Read(hash)
			if err != nil {
				return fmt.Errorf("could not read blob: %v", err)
			}
		} else {
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				return fmt.Errorf("could not create new http request: %v", err)
			}
			req.Header.Set("Accept-Encoding", "*")

			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("could not download dist-git file: %v", err)
			}

			body, err = ioutil.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("could not read the whole dist-git file: %v", err)
			}
			err = resp.Body.Close()
			if err != nil {
				return fmt.Errorf("could not close body handle: %v", err)
			}
		}

		hasher := pd.CompareHash(body, hash)
		if hasher == nil {
			return fmt.Errorf("checksum in metadata does not match dist-git file")
		}

		err = os.MkdirAll(filepath.Join(dir, filepath.Dir(path)), 0755)
		if err != nil {
			return fmt.Errorf("could create all directories")
		}

		f, err := os.Create(filepath.Join(dir, path))
		if err != nil {
			return fmt.Errorf("could not open file pointer: %v", err)
		}

		_, err = f.Write(body)
		if err != nil {
			return fmt.Errorf("could not copy dist-git file to in-tree: %v", err)
		}
		_ = f.Close()
	}

	return nil
}
