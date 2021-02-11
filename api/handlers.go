package api

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/NebulousLabs/skynet-accounts/database"
	"github.com/NebulousLabs/skynet-accounts/metafetcher"
	"gitlab.com/NebulousLabs/errors"

	"github.com/julienschmidt/httprouter"
)

// userHandler returns information about an existing user and create it if it
// doesn't exist.
func (api *API) userHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, true)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteJSON(w, u)
}

// userUploadsHandler returns all uploads made by the current user.
func (api *API) userUploadsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		api.WriteError(w, err, http.StatusUnauthorized)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, true)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	if err = req.ParseForm(); err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
	}
	offset, err1 := fetchOffset(req.Form)
	pageSize, err2 := fetchPageSize(req.Form)
	if err = errors.Compose(err1, err2); err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
	}
	ups, total, err := api.staticDB.UploadsByUser(req.Context(), *u, offset, pageSize)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
	}
	response := database.UploadsResponseDTO{
		Items:    ups,
		Offset:   offset,
		PageSize: pageSize,
		Count:    total,
	}
	api.WriteJSON(w, response)
}

// userDownloadsHandler returns all downloads made by the current user.
func (api *API) userDownloadsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, true)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	if err = req.ParseForm(); err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
	}
	offset, err1 := fetchOffset(req.Form)
	pageSize, err2 := fetchPageSize(req.Form)
	if err = errors.Compose(err1, err2); err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
	}
	downs, total, err := api.staticDB.DownloadsByUser(req.Context(), *u, offset, pageSize)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
	}
	response := database.DownloadsResponseDTO{
		Items:    downs,
		Offset:   offset,
		PageSize: pageSize,
		Count:    total,
	}
	api.WriteJSON(w, response)
}

// trackUploadHandler registers a new upload in the system.
func (api *API) trackUploadHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	sl := ps.ByName("skylink")
	if sl == "" {
		api.WriteError(w, errors.New("missing parameter 'skylink'"), http.StatusBadRequest)
		return
	}
	skylink, err := api.staticDB.Skylink(req.Context(), sl)
	if errors.Contains(err, database.ErrInvalidSkylink) {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, true)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	_, err = api.staticDB.UploadCreate(req.Context(), *u, *skylink)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	if skylink.Size == 0 {
		// Zero size means that we haven't fetched the skyfile's size yet.
		// Queue the skylink to have its meta data fetched and updated in the
		// DB, as well as the user's used space to be updated.
		api.staticMF.Queue <- metafetcher.Message{
			UserID:    u.ID,
			SkylinkID: skylink.ID,
		}
	} else {
		err = api.staticDB.UserUpdateUsedStorage(req.Context(), u.ID, skylink.Size)
		if err != nil {
			// Log the error but return success - the record will be corrected
			// later when we rescan the user's used space.
			api.staticLogger.Debug("Failed to update user's used space:", err)
		}
	}
	api.WriteSuccess(w)
}

// trackDownloadHandler registers a new download in the system.
func (api *API) trackDownloadHandler(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
	sub, _, _, err := tokenFromContext(req)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	sl := ps.ByName("skylink")
	if sl == "" {
		api.WriteError(w, errors.New("missing parameter 'skylink'"), http.StatusBadRequest)
		return
	}
	skylink, err := api.staticDB.Skylink(req.Context(), sl)
	if errors.Contains(err, database.ErrInvalidSkylink) {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	u, err := api.staticDB.UserBySub(req.Context(), sub, true)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	_, err = api.staticDB.DownloadCreate(req.Context(), *u, *skylink)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// proxyToKratos proxies this request to Kratos without interfering with it.
func (api *API) proxyToKratos(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	schema := req.URL.Scheme
	if schema == "" {
		schema = "http"
	}
	originalURL, err := url.Parse(req.RequestURI)
	if err != nil {
		// ...
	}
	strippedPath := strings.ReplaceAll(originalURL.Path, "/.ory/kratos/public", "")
	u, _ := url.Parse(schema + "://" + kratosAddr)

	// create the reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(u)

	//REQUEST BEFORE: &{Method:POST URL:/.ory/kratos/public/self-service/login/methods/password?flow=d2667107-5dd6-423c-be8d-e7cddf49b50e Proto:HTTP/1.1 ProtoMajor:1 ProtoMinor:1 Header:map[Accept:[text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8] Accept-Encoding:[gzip, deflate, br] Accept-Language:[en-GB,bg;q=0.8,en;q=0.6,en-US;q=0.4,de-CH;q=0.2] Cache-Control:[no-cache] Content-Length:[173] Content-Type:[application/x-www-form-urlencoded] Cookie:[csrf_token=O5phwVCUnourOJaS1gsQ1wbQqzriABME6dEF/fuLIzA=] Dnt:[1] Origin:[https://secure.siasky.xyz] Pragma:[no-cache] Referer:[https://secure.siasky.xyz/auth/login?flow=d2667107-5dd6-423c-be8d-e7cddf49b50e] Upgrade-Insecure-Requests:[1] User-Agent:[Sia-Agent Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:77.0) Gecko/20100101 Firefox/77.0] X-Forwarded-For:[31.10.147.72, 10.10.10.30] X-Forwarded-Proto:[https]] Body:0xc0000ae3c0 GetBody:<nil> ContentLength:173 TransferEncoding:[] Close:false Host:oathkeeper:4455 Form:map[] PostForm:map[] MultipartForm:<nil> Trailer:map[] RemoteAddr:10.10.10.83:33232 RequestURI:/.ory/kratos/public/self-service/login/methods/password?flow=d2667107-5dd6-423c-be8d-e7cddf49b50e TLS:<nil> Cancel:<nil> Response:<nil> ctx:0xc0000ae400}

	fmt.Printf("REQUEST BEFORE: %+v\n\n", req)

	// Update the headers to allow for SSL redirection
	//req.RequestURI = strippedPath
	req.URL.Host = u.Host
	req.URL.Scheme = u.Scheme
	req.URL.Path = strippedPath
	//req.URL.RawPath = strippedPath
	req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
	req.Host = u.Host

	fmt.Printf("REQUEST AFTER: %+v\n\n", req)

	// Note that ServeHttp is non blocking and uses a go routine under the hood
	proxy.ServeHTTP(w, req)
}

// fetchOffset extracts the offset from the params and validates its value.
func fetchOffset(form url.Values) (int, error) {
	offset, _ := strconv.Atoi(form.Get("offset"))
	if offset < 0 {
		return 0, errors.New("Invalid offset")
	}
	return offset, nil
}

// fetchPageSize extracts the page size from the params and validates its value.
func fetchPageSize(form url.Values) (int, error) {
	pageSize, _ := strconv.Atoi(form.Get("pageSize"))
	if pageSize < 0 {
		return 0, errors.New("Invalid page size")
	}
	if pageSize == 0 {
		pageSize = database.DefaultPageSize
	}
	return pageSize, nil
}
