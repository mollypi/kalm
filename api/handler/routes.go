package handler

import (
	client2 "github.com/kalmhq/kalm/api/client"
	"github.com/kalmhq/kalm/api/resources"
	"github.com/labstack/echo/v4"
	"strings"
)

func (h *ApiHandler) handleListAllRoutes(c echo.Context) error {
	list, err := h.resourceManager.GetHttpRoutes("")
	list = h.filterAuthorizedHttpRoutes(c, list)

	if err != nil {
		return err
	}

	return c.JSON(200, list)
}

func (h *ApiHandler) handleListRoutes(c echo.Context) error {
	list, err := h.resourceManager.GetHttpRoutes(c.Param("namespace"))
	list = h.filterAuthorizedHttpRoutes(c, list)

	if err != nil {
		return err
	}

	return c.JSON(200, list)
}

func (h *ApiHandler) handleCreateRoute(c echo.Context) (err error) {
	var route *resources.HttpRoute

	if route, err = getHttpRouteFromContext(c); err != nil {
		return err
	}

	if !h.CanOperateHttpRoute(getCurrentUser(c), "edit", route) {
		return resources.InsufficientPermissionsError
	}

	if route, err = h.resourceManager.CreateHttpRoute(route); err != nil {
		return err
	}

	return c.JSON(201, route)
}

func (h *ApiHandler) handleUpdateRoute(c echo.Context) (err error) {
	var route *resources.HttpRoute

	if route, err = getHttpRouteFromContext(c); err != nil {
		return err
	}

	if !h.CanOperateHttpRoute(getCurrentUser(c), "edit", route) {
		return resources.InsufficientPermissionsError
	}

	if route, err = h.resourceManager.UpdateHttpRoute(route); err != nil {
		return err
	}

	return c.JSON(200, route)
}

func (h *ApiHandler) handleDeleteRoute(c echo.Context) (err error) {
	route, err := h.resourceManager.GetHttpRoute(c.Param("namespace"), c.Param("name"))

	if err != nil {
		return nil
	}

	if !h.CanOperateHttpRoute(getCurrentUser(c), "edit", route) {
		return resources.InsufficientPermissionsError
	}

	if err = h.resourceManager.DeleteHttpRoute(route.Namespace, route.Name); err != nil {
		return err
	}

	return c.NoContent(200)
}

func getHttpRouteFromContext(c echo.Context) (*resources.HttpRoute, error) {
	var route resources.HttpRoute

	if err := c.Bind(&route); err != nil {
		return nil, err
	}

	return &route, nil
}

func (h *ApiHandler) filterAuthorizedHttpRoutes(c echo.Context, records []*resources.HttpRoute) []*resources.HttpRoute {
	l := len(records)

	for i := 0; i < l; i++ {
		if !h.CanOperateHttpRoute(getCurrentUser(c), "view", records[i]) {
			records[l-1], records[i] = records[i], records[l-1]
			i--
			l--
		}
	}

	return records[:l]
}

func (h *ApiHandler) CanOperateHttpRoute(client *client2.ClientInfo, action string, route *resources.HttpRoute) bool {
	for _, dest := range route.HttpRouteSpec.Destinations {
		parts := strings.Split(dest.Host, ".")

		if len(parts) == 0 {
			return false
		}

		var ns string
		if len(parts) == 1 {
			ns = route.Namespace
		} else {
			ns = parts[1]
		}

		if action == "view" {
			if !h.clientManager.CanViewNamespace(client, ns) {
				return false
			}
		} else if action == "edit" {
			if !h.clientManager.CanEditNamespace(client, ns) {
				return false
			}
		}
	}

	return true
}
