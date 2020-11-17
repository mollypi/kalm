package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"sync"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/kalmhq/kalm/api/log"
	"github.com/kalmhq/kalm/api/rbac"
	"github.com/kalmhq/kalm/api/resources"
	"github.com/kalmhq/kalm/controller/api/v1alpha1"
	"github.com/kalmhq/kalm/controller/controllers"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

type StandardClientManager struct {
	*BaseClientManager

	PolicyAdapter *rbac.StringPolicyAdapter

	ClusterConfig *rest.Config

	// Access tokens, roleBindings, applications are rarely changed.
	// It is efficient to hold all roles and access tokens in memory to authorize requests.
	mut           *sync.RWMutex
	Applications  map[string]*coreV1.Namespace
	AccessTokens  map[string]*v1alpha1.AccessToken
	Tenants       map[string]*v1alpha1.Tenant
	RoleBindings  map[string]*v1alpha1.RoleBinding
	StopWatchChan chan struct{}
}

func BuildClusterRolePolicies() string {
	return `
# cluster role policies
p, role_cluster_viewer, view, */*, */*
p, role_cluster_editor, edit, */*, */*
p, role_cluster_owner, manage, */*, */*
g, role_cluster_editor, role_cluster_viewer
g, role_cluster_owner, role_cluster_editor
`
}

func BuildRolePoliciesForTenantAndNamespace(tenant, name string) string {
	t := template.Must(template.New("policy").Parse(`
# {{ .name }} application role policies
p, role_{{ .tenant }}_{{ .name }}_viewer, view, {{ .tenant }}/{{ .name }}, */*
p, role_{{ .tenant }}_{{ .name }}_viewer, view, */*, storageClasses/*
p, role_{{ .tenant }}_{{ .name }}_editor, edit, {{ .tenant }}/{{ .name }}, */*
p, role_{{ .tenant }}_{{ .name }}_editor, view, {{ .tenant }}/*, registries/*
p, role_{{ .tenant }}_{{ .name }}_owner, manage, {{ .tenant }}/{{ .name }}, */*

g, role_{{ .tenant }}_{{ .name }}_editor, role_{{ .tenant }}_{{ .name }}_viewer
g, role_{{ .tenant }}_{{ .name }}_owner, role_{{ .tenant }}_{{ .name }}_editor
`))

	strBuffer := &strings.Builder{}
	_ = t.Execute(strBuffer, map[string]string{"name": name, "tenant": tenant})

	return strBuffer.String()
}

func BuildTenantOwnerPolicies(tenant string) string {
	t := template.Must(template.New("policy").Parse(`
# {{ .name }} tenant owner policies
p, tenant_{{ .tenant }}_owner, view, */*, storageClasses/*

p, tenant_{{ .tenant }}_owner, view, {{ .tenant }}/*, */*
p, tenant_{{ .tenant }}_owner, edit, {{ .tenant }}/*, */*
p, tenant_{{ .tenant }}_owner, manage, {{ .tenant }}/*, */*
`))

	strBuffer := &strings.Builder{}
	_ = t.Execute(strBuffer, map[string]string{"tenant": tenant})

	return strBuffer.String()
}

func GetPoliciesFromAccessToken(accessToken *resources.AccessToken) [][]string {
	var res = [][]string{}
	for _, rule := range accessToken.Rules {

		var scope string

		// TODO: should work, But not clean.
		if accessToken.Tenant == "global" {
			scope = fmt.Sprintf("*/%s", rule.Namespace)
		} else {
			scope = fmt.Sprintf("%s/%s", accessToken.Tenant, rule.Namespace)
		}

		obj := fmt.Sprintf("%s/%s", rule.Kind, rule.Name)

		res = append(res, []string{
			ToSafeSubject(accessToken.Name, v1alpha1.SubjectTypeUser),
			string(rule.Verb),
			scope,
			obj,
		})
	}

	return res
}

func roleValueToPolicyValue(tenant, ns, role string) string {
	switch role {
	case v1alpha1.ClusterRoleViewer:
		return "role_cluster_viewer"
	case v1alpha1.ClusterRoleEditor:
		return "role_cluster_editor"
	case v1alpha1.ClusterRoleOwner:
		return "role_cluster_owner"
	default:
		return fmt.Sprintf("role_%s_%s_%s", tenant, ns, role)
	}

}

func (m *StandardClientManager) UpdatePolicies() {
	var sb strings.Builder

	sb.WriteString(BuildClusterRolePolicies())

	for _, application := range m.Applications {
		if application.Labels == nil || application.Labels[controllers.KalmEnableLabelName] != controllers.KalmEnableLabelValue {
			continue
		}

		// TODO: error
		tenantName, _ := v1alpha1.GetTenantNameFromObj(application)

		sb.WriteString(BuildRolePoliciesForTenantAndNamespace(tenantName, application.Name))
	}

	for _, tenant := range m.Tenants {
		sb.WriteString(BuildTenantOwnerPolicies(tenant.Name))

		for _, owner := range tenant.Spec.Owners {
			sb.WriteString(
				fmt.Sprintf("g, %s, tenant_%s_owner\n", ToSafeSubject(owner, v1alpha1.SubjectTypeUser), tenant.Name),
			)
		}
	}

	for i := range m.AccessTokens {
		accessToken := m.AccessTokens[i]

		if accessToken.Spec.ExpiredAt != nil && accessToken.Spec.ExpiredAt.Time.Before(time.Now()) {
			continue
		}

		tenantName, err := v1alpha1.GetTenantNameFromObj(accessToken)

		if err != nil {
			log.Error(fmt.Sprintf("Can't find tenantName from AccessToken %s", accessToken.Name))
			continue
		}

		sb.WriteString(fmt.Sprintf("# policies for access token %s\n", accessToken.Name))

		for _, policy := range GetPoliciesFromAccessToken(&resources.AccessToken{Name: accessToken.Name, Tenant: tenantName, AccessTokenSpec: &accessToken.Spec}) {
			sb.WriteString(
				fmt.Sprintf(
					"p, %s, %s, %s, %s\n",
					policy[0],
					policy[1],
					policy[2],
					policy[3],
				),
			)
		}
	}

	for _, roleBinding := range m.RoleBindings {
		if roleBinding.Spec.ExpiredAt != nil && roleBinding.Spec.ExpiredAt.Time.Before(time.Now()) {
			continue
		}

		// TODO: handle error
		tenantName, _ := v1alpha1.GetTenantNameFromObj(roleBinding)

		sb.WriteString(fmt.Sprintf("# policies for rolebinding %s\n", roleBinding.Name))
		sb.WriteString(fmt.Sprintf(
			"g, %s, %s\n",
			ToSafeSubject(roleBinding.Spec.Subject, roleBinding.Spec.SubjectType),
			roleValueToPolicyValue(tenantName, roleBinding.Namespace, roleBinding.Spec.Role)),
		)
	}

	m.PolicyAdapter.SetPoliciesString(sb.String())

	if err := m.RBACEnforcer.LoadPolicy(); err != nil {
		// the policy is important. Stale policies can be harmful.
		panic(err)
	}
}

func (m *StandardClientManager) GetDefaultClusterConfig() *rest.Config {
	return m.ClusterConfig
}

func (m *StandardClientManager) GetClientInfoFromToken(tokenString string) (*ClientInfo, error) {
	m.mut.RLock()
	defer m.mut.RUnlock()

	accessToken, ok := m.AccessTokens[v1alpha1.GetAccessTokenNameFromToken(tokenString)]

	if !ok {
		return nil, errors.NewUnauthorized("access token not exist")
	}

	if accessToken.Spec.ExpiredAt != nil && accessToken.Spec.ExpiredAt.Time.Before(time.Now()) {
		return nil, errors.NewUnauthorized("access token is expired")
	}

	tenantName, err := v1alpha1.GetTenantNameFromObj(accessToken)

	if err != nil {
		return nil, err
	}

	clientInfo := &ClientInfo{
		Cfg:           m.ClusterConfig,
		Name:          accessToken.Name,
		Email:         accessToken.Name,
		EmailVerified: false,
		Tenant:        tenantName,
		Tenants:       []string{tenantName},
		Groups:        []string{},
	}

	return clientInfo, nil
}

func (m *StandardClientManager) SetImpersonation(clientInfo *ClientInfo, rawImpersonation string) {
	if rawImpersonation == "" {
		return
	}

	if m.CanManageCluster(clientInfo) {
		impersonation, impersonationType, err := parseImpersonationString(rawImpersonation)

		if err == nil {
			clientInfo.Impersonation = impersonation
			clientInfo.ImpersonationType = impersonationType
		} else {
			log.Error("parse impersonation raw string failed", zap.Error(err))
		}
	}

	if m.CanManageScope(clientInfo, clientInfo.Tenant+"/*") {
		impersonation, impersonationType, err := parseImpersonationString(rawImpersonation)

		if err != nil {
			log.Error("parse impersonation raw string failed", zap.Error(err))
			return
		}

		if impersonation == "" || impersonationType == "" {
			return
		}

		policies, err := m.GetRBACEnforcer().GetImplicitPermissionsForUser(ToSafeSubject(impersonation, impersonationType))

		if err != nil {
			log.Error("get implicit permissions for error", zap.String("user", ToSafeSubject(impersonation, impersonationType)), zap.Error(err))
			return
		}

		for _, policy := range policies {
			if !m.Can(clientInfo, policy[1], policy[2], policy[3]) {
				// Can't impersonate. The goal user has at least one permission that current user don't have
				log.Debug(
					"Impersonate failed",
					zap.String("currentUser", clientInfo.Email),
					zap.String("goal user", ToSafeSubject(impersonation, impersonationType)),
					zap.String("action", policy[1]),
					zap.String("scope", policy[2]),
					zap.String("object", policy[3]),
				)
				return
			}
		}

		clientInfo.Impersonation = impersonation
		clientInfo.ImpersonationType = impersonationType
	}
}

func (m *StandardClientManager) GetClientInfoFromContext(c echo.Context) (*ClientInfo, error) {
	// If the Authorization Header is not empty, use the bearer token as k8s token.
	if token := extractAuthTokenFromClientRequestContext(c); token != "" {
		clientInfo, err := m.GetClientInfoFromToken(token)
		if err != nil {
			return nil, err
		}

		m.SetImpersonation(clientInfo, c.Request().Header.Get("Kalm-Impersonation"))
		return clientInfo, nil
	}

	// And the kalm-sso-userinfo header is not empty.
	// This header will be removed at ingress route level. Only auth proxy can set this header, So it's safe to trust this value.
	if c.Request().Header.Get("Kalm-Sso-Userinfo") != "" {
		claimsBytes, err := base64.RawStdEncoding.DecodeString(c.Request().Header.Get("Kalm-Sso-Userinfo"))

		if err != nil {
			return nil, err
		}

		var clientInfo ClientInfo

		err = json.Unmarshal(claimsBytes, &clientInfo)

		if err != nil {
			return nil, err
		}

		clientInfo.Cfg = m.ClusterConfig
		clientInfo.Impersonation = ""

		if clientInfo.Groups == nil {
			clientInfo.Groups = []string{}
		}

		if clientInfo.Tenants == nil {
			clientInfo.Tenants = []string{}
		}

		switch len(clientInfo.Tenants) {
		case 0:
			clientInfo.Tenant = ""
		case 1:
			clientInfo.Tenant = clientInfo.Tenants[0]
		default:

			m := make(map[string]struct{})

			for _, tenant := range clientInfo.Tenants {
				parts := strings.Split(tenant, "/")

				if len(parts) != 2 {
					continue
				}

				// TODO check part[0] is current cluster

				m[parts[1]] = struct{}{}
			}

			cookie, err := c.Cookie("selected-tenant")

			if err == nil {
				if _, ok := m[cookie.Value]; ok {
					clientInfo.Tenant = cookie.Value
				}
			}

			if clientInfo.Tenant == "" {
				lowercaseHost := strings.ToLower(c.Request().Host)

				if strings.HasSuffix(lowercaseHost, "kapp.live") || strings.HasSuffix(lowercaseHost, "kalm.dev") {
					// x.y.z.tenantName.region.kalm.dev
					// parts length = 7
					// tenantName = parts[7-4]
					parts := strings.Split(c.Request().Host, ".")

					if len(parts) >= 4 {
						tenantName := parts[len(parts)-4]
						if _, ok := m[tenantName]; ok {
							clientInfo.Tenant = tenantName
						}
					}
				}
			}
		}

		m.SetImpersonation(&clientInfo, c.Request().Header.Get("Kalm-Impersonation"))
		return &clientInfo, nil
	}

	return nil, errors.NewUnauthorized("")
}

// Since the token is validated by api server, so we don't need to valid the token again here.
func tryToParseEntityFromToken(tokenString string) string {
	if tokenString == "" {
		return "unknown"
	}

	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})

	if err != nil {
		return "token"
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		return claims["sub"].(string)
	}

	return "token"
}

func NewStandardClientManager(cfg *rest.Config) *StandardClientManager {
	policyAdapter := rbac.NewStringPolicyAdapter(``)

	manager := &StandardClientManager{
		BaseClientManager: NewBaseClientManager(policyAdapter),
		PolicyAdapter:     policyAdapter,
		ClusterConfig:     cfg,
		mut:               &sync.RWMutex{},
		Tenants:           make(map[string]*v1alpha1.Tenant),
		Applications:      make(map[string]*coreV1.Namespace),
		AccessTokens:      make(map[string]*v1alpha1.AccessToken),
		RoleBindings:      make(map[string]*v1alpha1.RoleBinding),
		StopWatchChan:     make(chan struct{}),
	}

	go setupResourcesWatcher(cfg, manager)
	go policyRegenerateLoop(manager)

	return manager
}

// Run per minute to remove expired access tokens and role bindings
func policyRegenerateLoop(manager *StandardClientManager) {
	for {
		manager.mut.Lock()
		manager.UpdatePolicies()
		manager.mut.Unlock()
		time.Sleep(1 * time.Minute)
	}
}

func setupResourcesWatcher(cfg *rest.Config, manager *StandardClientManager) {
	informerCache, err := cache.New(cfg, cache.Options{})

	if err != nil {
		log.Error("new cache error", zap.Error(err))
		panic(err)
	}

	if informer, err := informerCache.GetInformer(context.Background(), &coreV1.Namespace{}); err == nil {
		informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				manager.mut.Lock()
				defer manager.mut.Unlock()
				if namespace, ok := obj.(*coreV1.Namespace); ok {
					manager.Applications[namespace.Name] = namespace
					manager.UpdatePolicies()
				}
			},
			DeleteFunc: func(obj interface{}) {
				manager.mut.Lock()
				defer manager.mut.Unlock()
				if namespace, ok := obj.(*coreV1.Namespace); ok {
					delete(manager.Applications, namespace.Name)
					manager.UpdatePolicies()
				}
			},
			UpdateFunc: func(oldObj, obj interface{}) {
				manager.mut.Lock()
				defer manager.mut.Unlock()
				if namespace, ok := obj.(*coreV1.Namespace); ok {
					manager.Applications[namespace.Name] = namespace
					manager.UpdatePolicies()
				}
			},
		})
	} else {
		log.Error("get informer error", zap.Error(err))
		panic(err)
	}

	if informer, err := informerCache.GetInformer(context.Background(), &v1alpha1.RoleBinding{}); err == nil {
		informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				manager.mut.Lock()
				defer manager.mut.Unlock()
				if roleBinding, ok := obj.(*v1alpha1.RoleBinding); ok {
					manager.RoleBindings[getNamespacedName(roleBinding.ObjectMeta)] = roleBinding
					manager.UpdatePolicies()
				}
			},
			DeleteFunc: func(obj interface{}) {
				manager.mut.Lock()
				defer manager.mut.Unlock()
				if roleBinding, ok := obj.(*v1alpha1.RoleBinding); ok {
					delete(manager.RoleBindings, getNamespacedName(roleBinding.ObjectMeta))
					manager.UpdatePolicies()
				}
			},
			UpdateFunc: func(oldObj, obj interface{}) {
				manager.mut.Lock()
				defer manager.mut.Unlock()
				if roleBinding, ok := obj.(*v1alpha1.RoleBinding); ok {
					manager.RoleBindings[getNamespacedName(roleBinding.ObjectMeta)] = roleBinding
					manager.UpdatePolicies()
				}
			},
		})
	} else {
		log.Error("get informer error", zap.Error(err))
		panic(err)
	}

	if informer, err := informerCache.GetInformer(context.Background(), &v1alpha1.AccessToken{}); err == nil {
		informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				manager.mut.Lock()
				defer manager.mut.Unlock()
				if accessToken, ok := obj.(*v1alpha1.AccessToken); ok {
					manager.AccessTokens[accessToken.Name] = accessToken
					manager.UpdatePolicies()
				}
			},
			DeleteFunc: func(obj interface{}) {
				manager.mut.Lock()
				defer manager.mut.Unlock()
				if accessToken, ok := obj.(*v1alpha1.AccessToken); ok {
					delete(manager.AccessTokens, accessToken.Name)
					manager.UpdatePolicies()
				}
			},
			UpdateFunc: func(oldObj, obj interface{}) {
				manager.mut.Lock()
				defer manager.mut.Unlock()
				if accessToken, ok := obj.(*v1alpha1.AccessToken); ok {
					manager.AccessTokens[accessToken.Name] = accessToken
					manager.UpdatePolicies()
				}
			},
		})
	} else {
		log.Error("get informer error", zap.Error(err))
		panic(err)
	}

	if informer, err := informerCache.GetInformer(context.Background(), &v1alpha1.Tenant{}); err == nil {
		informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				manager.mut.Lock()
				defer manager.mut.Unlock()

				if tenant, ok := obj.(*v1alpha1.Tenant); ok {
					manager.Tenants[tenant.Name] = tenant
					manager.UpdatePolicies()
				}
			},
			DeleteFunc: func(obj interface{}) {
				manager.mut.Lock()
				defer manager.mut.Unlock()

				if tenant, ok := obj.(*v1alpha1.Tenant); ok {
					delete(manager.Tenants, tenant.Name)
					manager.UpdatePolicies()
				}
			},
			UpdateFunc: func(oldObj, obj interface{}) {
				manager.mut.Lock()
				defer manager.mut.Unlock()
				if tenant, ok := obj.(*v1alpha1.Tenant); ok {
					manager.Tenants[tenant.Name] = tenant
					manager.UpdatePolicies()
				}
			},
		})
	} else {
		log.Error("get informer error", zap.Error(err))
		panic(err)
	}

	informerCache.Start(manager.StopWatchChan)
}

func getNamespacedName(metaObj metaV1.ObjectMeta) string {
	return fmt.Sprintf("%s-%s", metaObj.Namespace, metaObj.Name)
}
