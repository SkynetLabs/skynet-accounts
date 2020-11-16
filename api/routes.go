package api

// buildHTTPRoutes registers all HTTP routes and their handlers.
func (api *API) buildHTTPRoutes() {
	// TODO Rate-limit these from the start to prevent brute force and other forms of abuse.
	api.Router.POST("/login", api.userLoginHandler)
	api.Router.POST("/user", api.userHandlerPOST)
	api.Router.GET("/user/", api.userHandlerGET)
	api.Router.PUT("/user/:id", api.userHandlerPUT)
	api.Router.POST("/user/:id/password", api.userChangePasswordHandler)
	api.Router.POST("/user/:id/password/reset/request", api.userPasswordResetRequestHandler)
	api.Router.POST("/user/:id/password/reset/verify", api.userPasswordResetCompleteHandler)
	api.Router.POST("/user/:id/password/reset/complete", api.userPasswordResetCompleteHandler)
}
