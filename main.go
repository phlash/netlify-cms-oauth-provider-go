package main

import (
	// Refer to dotenv package first, to ensure it loads any .env settings before other init() functions try and use 'em.. isn't Golang great?
	"./dotenv"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"crypto/hmac"
	"crypto/sha1"
	"io/ioutil"
	"encoding/hex"
	"strings"
	"bytes"

	"github.com/gorilla/pat"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/bitbucket"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/gitlab"

)

var (
	host = "localhost:3000"
	base = host
)

const (
	script = `<!DOCTYPE html><html><head><script>
  if (!window.opener) {
	window.opener = {
	  postMessage: function(action, origin) {
		console.log(action, origin);
	  }
	}
  }
  (function(status, provider, result) {
	function recieveMessage(e) {
	  console.log("Recieve message:", e);
	  // send message to main window with da app
	  window.opener.postMessage("authorization:" + provider + ":" + status + ":" + result, "*");
	}
	window.addEventListener("message", recieveMessage, false);
	// Start handshare with parent
	console.log("Sending message:", provider)
	window.opener.postMessage(
	  "authorizing:" + provider,
	  "*"
	);
  })("%s", "%s", '%s')
  </script></head><body></body></html>`
)

// GET /
func handleMain(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-Type", "text/html; charset=utf-8")
	res.WriteHeader(http.StatusOK)
	res.Write([]byte(``))
}

// GET /auth Page  redirecting after provider get param
func handleAuth(res http.ResponseWriter, req *http.Request) {
	url := fmt.Sprintf("%s/auth/%s", base, req.FormValue("provider"))
	fmt.Printf("redirect to %s\n", url)
	http.Redirect(res, req, url, http.StatusTemporaryRedirect)
}

// GET /auth/provider  Initial page redirecting by provider
func handleAuthProvider(res http.ResponseWriter, req *http.Request) {
	gothic.BeginAuthHandler(res, req)
}

// GET /callback/{provider}  Called by provider after authorization is granted
func handleCallbackProvider(res http.ResponseWriter, req *http.Request) {
	var (
		status string
		result string
	)
	provider, errProvider := gothic.GetProviderName(req)
	user, errAuth := gothic.CompleteUserAuth(res, req)
	status = "error"
	if errProvider != nil {
		fmt.Printf("provider failed with '%s'\n", errProvider)
		result = fmt.Sprintf("%s", errProvider)
	} else if errAuth != nil {
		fmt.Printf("auth failed with '%s'\n", errAuth)
		result = fmt.Sprintf("%s", errAuth)
	} else {
		fmt.Printf("Logged in as %s user: %s (%s)\n", user.Provider, user.Email, user.AccessToken)
		status = "success"
		result = fmt.Sprintf(`{"token":"%s", "provider":"%s"}`, user.AccessToken, user.Provider)
	}
	res.Header().Set("Content-Type", "text/html; charset=utf-8")
	res.WriteHeader(http.StatusOK)
	res.Write([]byte(fmt.Sprintf(script, status, provider, result)))
}

// GET /refresh
func handleRefresh(res http.ResponseWriter, req *http.Request) {
	fmt.Printf("refresh with '%s'\n", req)
	res.Write([]byte(""))
}

// GET /success
func handleSuccess(res http.ResponseWriter, req *http.Request) {
	fmt.Printf("success with '%s'\n", req)
	res.Write([]byte(""))
}

// POST /callback/deploy
func handleDeploy(res http.ResponseWriter, req *http.Request) {
	fmt.Printf("deploy from github..");
	// Check signature in webhook
	if hookSecret, ok := os.LookupEnv("GITHUB_HOOK_SECRET"); ok {
		if body, ok := isValidSignature(req, hookSecret); ok {
			// Go ahead and run a deploy, passing on the webhook content
			if hookExec, ok := os.LookupEnv("GITHUB_HOOK_EXEC"); ok {
				os.Setenv("GITHUB_EVENT", req.Header.Get("X-GitHub-Event"))
				os.Setenv("GITHUB_DELIVERY", req.Header.Get("X-GitHub-Delivery"))
				cmd := exec.Command(hookExec)
				cmd.Stdin = bytes.NewBuffer(body)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				err := cmd.Run()
				if err != nil {
					fmt.Printf("unable to run deploy command (%s): %v\n", hookExec, err)
				} else {
					fmt.Printf("ok\n")
				}
			} else {
				fmt.Printf("missing GITHUB_HOOK_EXEC env\n")
			}
		} else {
			fmt.Printf("invalid hook signature\n")
		}
	} else {
		fmt.Printf("no secret, skipping\n")
	}
	res.Write([]byte(""))
}

// https://stackoverflow.com/questions/53242837/validating-github-webhook-hmac-signature-in-go
func isValidSignature(r *http.Request, key string) ([]byte, bool) {
	// Assuming a non-empty header
	gotHash := strings.SplitN(r.Header.Get("X-Hub-Signature"), "=", 2)
	if gotHash[0] != "sha1" {
		return nil, false
	}
	defer r.Body.Close()

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("Cannot read the request body: %s\n", err)
		return nil, false
	}

	hash := hmac.New(sha1.New, []byte(key))
	if _, err := hash.Write(b); err != nil {
		fmt.Printf("Cannot compute the HMAC for request: %s\n", err)
		return b, false
	}

	expectedHash := hex.EncodeToString(hash.Sum(nil))
	// special .env setting to support testing..
	if t, ok := os.LookupEnv("GITHUB_HOOK_TEST"); ok {
		fmt.Printf("Test mode: allowing hook: %s\n", t)
		return b, true
	}
	return b, (gotHash[1] == expectedHash)
}


func init() {
	fmt.Printf(".env loaded: %t\n", dotenv.LoadDotenv)
	if hostEnv, ok := os.LookupEnv("HOST"); ok {
		host = hostEnv
	}
	if baseEnv, ok := os.LookupEnv("BASE"); ok {
		base = baseEnv
	}
	fmt.Printf("host=%s base=%s\n", host, base)
	var (
		gitlabProvider goth.Provider
	)
	if gitlabServer, ok := os.LookupEnv("GITLAB_SERVER"); ok {
		gitlabProvider = gitlab.NewCustomisedURL(
			os.Getenv("GITLAB_KEY"), os.Getenv("GITLAB_SECRET"),
			fmt.Sprintf("%s/callback/gitlab", base),
			fmt.Sprintf("https://%s/oauth/authorize", gitlabServer),
			fmt.Sprintf("https://%s/oauth/token", gitlabServer),
			fmt.Sprintf("https://%s/api/v3/user", gitlabServer),
		)
	} else {
		gitlabProvider = gitlab.New(
			os.Getenv("GITLAB_KEY"), os.Getenv("GITLAB_SECRET"),
			fmt.Sprintf("%s/callback/gitlab", base),
		)
	}
	goth.UseProviders(
		github.New(
			os.Getenv("GITHUB_KEY"), os.Getenv("GITHUB_SECRET"),
			fmt.Sprintf("%s/callback/github", base),
			"repo", // https://developer.github.com/apps/building-oauth-apps/understanding-scopes-for-oauth-apps/
		),
		bitbucket.New(
			os.Getenv("BITBUCKET_KEY"), os.Getenv("BITBUCKET_SECRET"),
			fmt.Sprintf("%s/callback//bitbucket", base),
		),
		gitlabProvider,
	)
}

func main() {
	router := pat.New()
	router.Post("/callback/deploy", handleDeploy)
	router.Get("/callback/{provider}", handleCallbackProvider)
	router.Get("/auth/{provider}", handleAuthProvider)
	router.Get("/auth", handleAuth)
	router.Get("/refresh", handleRefresh)
	router.Get("/success", handleSuccess)
	router.Get("/", handleMain)
	//
	http.Handle("/", router)
	//
	fmt.Printf("Started running on %s\n", host)
	fmt.Println(http.ListenAndServe(host, nil))
}
