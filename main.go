package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const packageDirectory string = "data/packages"

type requestResult struct {
	code    int
	content interface{}
	error   string
}

type requestHandler func(r *http.Request) requestResult

func JSONResponse(contentFunc requestHandler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")

		result := contentFunc(r)

		w.WriteHeader(result.code)

		content := result.content
		if result.code < 200 || result.code >= 400 {
			errorContent := map[string]string{"error": http.StatusText(result.code)}
			if result.error != "" {
				errorContent["message"] = result.error
			}

			content = errorContent
		}

		err := json.NewEncoder(w).Encode(content)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	})
}

func readComposerJson(composerFileContent []byte) (map[string]interface{}, error) {
	composerJson := map[string]interface{}{}

	if err := json.Unmarshal(composerFileContent, &composerJson); err != nil {
		return nil, err
	}

	return composerJson, nil
}

func packagesJsonHandler(r *http.Request) requestResult {
	// [Package name: [package version: composer.json]]
	var packages = make(map[string]map[string]interface{})

	packageVendors, _ := ioutil.ReadDir(packageDirectory)
	for _, packageVendor := range packageVendors {
		if !packageVendor.IsDir() {
			continue
		}

		packageVendorFolder := path.Join(packageDirectory, packageVendor.Name())
		packageNames, _ := ioutil.ReadDir(packageVendorFolder)
		for _, packageName := range packageNames {
			if !packageName.IsDir() {
				continue
			}

			packageNameFolder := path.Join(packageVendorFolder, packageName.Name())
			packageVersions, _ := ioutil.ReadDir(packageNameFolder)

			var packageVersionMap = make(map[string]interface{})
			for _, packageVersion := range packageVersions {
				composerJsonPath := path.Join(packageNameFolder, packageVersion.Name(), "composer.json")
				if _, err := os.Stat(composerJsonPath); os.IsNotExist(err) {
					continue
				}

				composerJsonFileContent, err := ioutil.ReadFile(composerJsonPath)
				// TODO: Handle different and throw error?
				if err != nil {
					continue
				}

				composerJson, err := readComposerJson(composerJsonFileContent)
				// TODO: Handle different and throw error?
				if err != nil {
					continue
				}

				packageUrl := url.URL{
					// TODO: Scheme should be dynamic
					Scheme: "http",
					Host:   r.Host,
					Path:   "package/" + packageVendor.Name() + "/" + packageName.Name() + "/" + packageVersion.Name(),
				}

				composerJson["dist"] = map[string]string{"url": packageUrl.String(), "type": "zip"}
				packageVersionMap[packageVersion.Name()] = composerJson
			}

			packages[packageVendor.Name()+"/"+packageName.Name()] = packageVersionMap
		}
	}

	return requestResult{code: http.StatusOK, content: map[string]interface{}{"packages": packages}}
}

func uploadPackageHandler(r *http.Request) requestResult {
	if r.Method != http.MethodPost {
		return requestResult{code: http.StatusMethodNotAllowed}
	}

	reqBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return requestResult{code: http.StatusInternalServerError, error: "Cannot read request body: " + err.Error()}
	}

	zipReader, err := zip.NewReader(bytes.NewReader(reqBody), int64(len(reqBody)))
	if err != nil {
		return requestResult{code: http.StatusInternalServerError, error: "Cannot read ZIP content: " + err.Error()}
	}

	var composerFileContent []byte
	for _, zipFile := range zipReader.File {
		if filepath.Base(zipFile.Name) == "composer.json" {
			// TODO: Add verification that we only consider the composer.json from the package root directory
			f, err := zipFile.Open()
			if err != nil {
				return requestResult{code: http.StatusInternalServerError, error: "Cannot read composer.json: " + err.Error()}
			}

			defer f.Close()

			fileContent, err := ioutil.ReadAll(f)
			if err != nil {
				return requestResult{code: http.StatusInternalServerError, error: "Cannot read composer.json: " + err.Error()}
			}

			composerFileContent = fileContent
			break
		}
	}

	if len(composerFileContent) == 0 {
		return requestResult{code: http.StatusBadRequest, error: "Cannot find composer.json in ZIP file."}
	}

	composerJson := map[string]interface{}{}
	err = json.Unmarshal(composerFileContent, &composerJson)
	if err != nil {
		return requestResult{code: http.StatusBadRequest, error: "Cannot decode composer.json: " + err.Error()}
	}

	packageName, ok := composerJson["name"].(string)
	if !ok {
		return requestResult{code: http.StatusBadRequest, error: "Cannot find package name in composer.json."}
	}

	packageVersion, ok := composerJson["version"].(string)
	if !ok {
		return requestResult{code: http.StatusBadRequest, error: "Cannot find package version in composer.json."}
	}

	// TODO: Handle existing package for that specific version
	packagePath := packageDirectory + "/" + packageName + "/" + packageVersion
	log.Println("Adding package: " + packagePath)
	err = os.MkdirAll(packagePath, os.ModePerm)
	if err != nil {
		return requestResult{code: http.StatusInternalServerError, error: "Cannot create package folder: " + err.Error()}
	}

	err = os.WriteFile(packagePath+"/package.zip", reqBody, 0755)
	if err != nil {
		return requestResult{code: http.StatusInternalServerError, error: "Cannot write file: " + err.Error()}
	}

	err = os.WriteFile(packagePath+"/composer.json", composerFileContent, 0644)
	if err != nil {
		return requestResult{code: http.StatusInternalServerError, error: "Cannot write file: " + err.Error()}
	}

	return requestResult{code: http.StatusOK, content: map[string]string{
		"message": "Created composer package " + packageName + ", version " + packageVersion,
	}}
}

func handlePackageRequest(w http.ResponseWriter, r *http.Request) {
	requestUrlParts := strings.Split(r.URL.Path, "/")

	if len(requestUrlParts) < 4 {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)

		return
	}

	packageVendor := requestUrlParts[2]
	packageName := requestUrlParts[3]
	packageVersion := requestUrlParts[4]

	packagePath := path.Join(packageDirectory, packageVendor, packageName, packageVersion, "package.zip")
	if _, err := os.Stat(packagePath); os.IsNotExist(err) {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)

		return
	}

	packageContent, err := ioutil.ReadFile(packagePath)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Add("Content-Disposition", "attachment; filename=\""+packageVendor+"-"+packageName+".zip\"")
	w.Header().Add("Content-Length", fmt.Sprint(len(packageContent)))
	w.WriteHeader(http.StatusOK)

	_, err = w.Write(packageContent)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}
}

type basicAuth struct {
	username string
	password string
}

func (auth *basicAuth) Authenticate(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if ok {
			usernameHash := sha256.Sum256([]byte(username))
			passwordHash := sha256.Sum256([]byte(password))
			expectedUsernameHash := sha256.Sum256([]byte(auth.username))
			expectedPasswordHash := sha256.Sum256([]byte(auth.password))

			usernameMatch := (subtle.ConstantTimeCompare(usernameHash[:], expectedUsernameHash[:]) == 1)
			passwordMatch := (subtle.ConstantTimeCompare(passwordHash[:], expectedPasswordHash[:]) == 1)

			if usernameMatch && passwordMatch {
				next.ServeHTTP(w, r)
				return
			}
		}

		w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}

func main() {
	mux := http.NewServeMux()

	userBasicAuth := basicAuth{username: os.Getenv("USER_AUTH_USERNAME"), password: os.Getenv("USER_AUTH_PASSWORD")}
	if userBasicAuth.username == "" {
		log.Fatal("User basic auth username must be provided in USER_AUTH_USERNAME")
	}
	if userBasicAuth.password == "" {
		log.Fatal("User basic auth password must be provided USER_AUTH_PASSWORD")
	}

	adminBasicAuth := basicAuth{username: os.Getenv("ADMIN_AUTH_USERNAME"), password: os.Getenv("ADMIN_AUTH_PASSWORD")}
	if adminBasicAuth.username == "" {
		log.Fatal("Admin basic auth username must be provided in ADMIN_AUTH_USERNAME")
	}
	if adminBasicAuth.password == "" {
		log.Fatal("Admin basic auth password must be provided ADMIN_AUTH_PASSWORD")
	}

	// General available routes
	mux.Handle("/packages.json", JSONResponse(packagesJsonHandler))

	// Password protected user routes
	mux.Handle("/package/", userBasicAuth.Authenticate(handlePackageRequest))

	// Password protected admin routes
	mux.Handle("/admin/upload", adminBasicAuth.Authenticate(JSONResponse(uploadPackageHandler)))

	log.Println("Listening to :3000")
	log.Fatal(http.ListenAndServe(":3000", mux))
}
