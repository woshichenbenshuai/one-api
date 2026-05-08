package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
)

type authenticatorCodeRequest struct {
	OTPCode string `json:"otp_code"`
}

func GetAuthenticatorStatus(c *gin.Context) {
	user, err := model.GetUserById(c.GetInt(ctxkey.Id), true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"enabled": user.AuthenticatorEnabled,
			"secret_configured": user.AuthenticatorSecret != "",
		},
	})
}

func SetupAuthenticator(c *gin.Context) {
	user, err := model.GetUserById(c.GetInt(ctxkey.Id), true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	secret, err := common.GenerateAuthenticatorSecret()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	user.AuthenticatorSecret = secret
	user.AuthenticatorEnabled = false
	if err := model.DB.Model(&model.User{Id: user.Id}).Updates(map[string]any{
		"authenticator_secret":  secret,
		"authenticator_enabled": false,
	}).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"secret": secret,
			"otpauth_uri": common.BuildAuthenticatorURI(config.SystemName, user.Username, secret),
		},
	})
}

func EnableAuthenticator(c *gin.Context) {
	var req authenticatorCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	user, err := model.GetUserById(c.GetInt(ctxkey.Id), true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	if user.AuthenticatorSecret == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Authenticator is not configured",
		})
		return
	}
	if !common.VerifyAuthenticatorCode(user.AuthenticatorSecret, req.OTPCode) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Authenticator verification code is invalid",
		})
		return
	}
	user.AuthenticatorEnabled = true
	if err := user.Update(false); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func DisableAuthenticator(c *gin.Context) {
	var req authenticatorCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	user, err := model.GetUserById(c.GetInt(ctxkey.Id), true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	if user.AuthenticatorSecret == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Authenticator is not configured",
		})
		return
	}
	if !common.VerifyAuthenticatorCode(user.AuthenticatorSecret, req.OTPCode) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Authenticator verification code is invalid",
		})
		return
	}
	user.AuthenticatorSecret = ""
	user.AuthenticatorEnabled = false
	if err := model.DB.Model(&model.User{Id: user.Id}).Updates(map[string]any{
		"authenticator_secret":  "",
		"authenticator_enabled": false,
	}).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}
