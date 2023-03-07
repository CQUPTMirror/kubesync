package manager

import (
	"github.com/gin-gonic/gin"
)

func contextErrorLogger(c *gin.Context) {
	errs := c.Errors.ByType(gin.ErrorTypeAny)
	if len(errs) > 0 {
		for _, err := range errs {
			runLog.Error(err, `"in request "%s %s: %s"`,
				c.Request.Method, c.Request.URL.Path,
				err.Error())
		}
	}
	// pass on to the next middleware in chain
	c.Next()
}
