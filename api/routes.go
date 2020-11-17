package api

// buildHTTPRoutes registers all HTTP routes and their handlers.
func (api *API) buildHTTPRoutes() {
	// TODO Rate-limit these from the start to prevent brute force and other forms of abuse.
	api.staticRouter.POST("/login", api.userLoginHandler)
	api.staticRouter.POST("/user", api.userHandlerPOST)
	api.staticRouter.GET("/user/:id", api.userHandlerGET)
	api.staticRouter.PUT("/user/:id", api.userHandlerPUT)
	api.staticRouter.POST("/user/:id/password", api.userChangePasswordHandler)
	api.staticRouter.POST("/user/:id/password/reset/request", api.userPasswordResetRequestHandler)
	api.staticRouter.POST("/user/:id/password/reset/verify", api.userPasswordResetCompleteHandler)
	api.staticRouter.POST("/user/:id/password/reset/complete", api.userPasswordResetCompleteHandler)
}
