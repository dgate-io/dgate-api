package resources

import (
	"errors"
	"sort"

	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/dgate-io/dgate-api/pkg/util/keylock"
	"github.com/dgate-io/dgate-api/pkg/util/linker"
	"github.com/dgate-io/dgate-api/pkg/util/safe"
	"github.com/dgate-io/dgate-api/pkg/util/tree/avl"
)

type avlTreeLinker[T any] avl.Tree[string, *linker.Link[string, safe.Ref[T]]]

// ResourceManager is a struct that handles all resources and their links between each other
type ResourceManager struct {
	namespaces  avlTreeLinker[spec.DGateNamespace]
	services    avlTreeLinker[spec.DGateService]
	domains     avlTreeLinker[spec.DGateDomain]
	modules     avlTreeLinker[spec.DGateModule]
	routes      avlTreeLinker[spec.DGateRoute]
	secrets     avlTreeLinker[spec.DGateSecret]
	collections avlTreeLinker[spec.DGateCollection]
	mutex       *keylock.KeyLock

	// sorting can be expensive, so we cache the sorted list of domains
	priorityDomainCache []*spec.DGateDomain
}

type Options func(*ResourceManager)

func NewManager(opts ...Options) *ResourceManager {
	rm := &ResourceManager{
		namespaces:  avl.NewTree[string, *linker.Link[string, safe.Ref[spec.DGateNamespace]]](),
		services:    avl.NewTree[string, *linker.Link[string, safe.Ref[spec.DGateService]]](),
		domains:     avl.NewTree[string, *linker.Link[string, safe.Ref[spec.DGateDomain]]](),
		modules:     avl.NewTree[string, *linker.Link[string, safe.Ref[spec.DGateModule]]](),
		routes:      avl.NewTree[string, *linker.Link[string, safe.Ref[spec.DGateRoute]]](),
		collections: avl.NewTree[string, *linker.Link[string, safe.Ref[spec.DGateCollection]]](),
		secrets:     avl.NewTree[string, *linker.Link[string, safe.Ref[spec.DGateSecret]]](),
		mutex:       keylock.NewKeyLock(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(rm)
		}
	}
	return rm
}

func WithDefaultNamespace(ns *spec.Namespace) Options {
	return func(rm *ResourceManager) {
		rm.AddNamespace(ns)
	}
}

/*
	Namespace functions
*/

func (rm *ResourceManager) GetNamespace(namespace string) (*spec.DGateNamespace, bool) {
	defer rm.mutex.RLock(namespace)()
	return rm.getNamespace(namespace)
}

func (rm *ResourceManager) getNamespace(namespace string) (*spec.DGateNamespace, bool) {
	if lk, ok := rm.namespaces.Find(namespace); !ok {
		return nil, false
	} else {
		return lk.Item().Read(), true
	}
}

func (rm *ResourceManager) NamespaceCountEquals(target int) bool {
	if target < 0 {
		panic("target must be greater than or equal to 0")
	}
	rm.namespaces.Each(func(_ string, lk *linker.Link[string, safe.Ref[spec.DGateNamespace]]) bool {
		target -= 1
		return target > 0
	})
	return target == 0
}

func (rm *ResourceManager) GetFirstNamespace() *spec.DGateNamespace {
	if _, nsLink, ok := rm.namespaces.RootKeyValue(); ok {
		return nsLink.Item().Read()
	}
	return nil
}

// GetNamespaces returns a list of all namespaces
func (rm *ResourceManager) GetNamespaces() []*spec.DGateNamespace {
	defer rm.mutex.RLockMain()()
	var namespaces []*spec.DGateNamespace
	rm.namespaces.Each(func(_ string, lk *linker.Link[string, safe.Ref[spec.DGateNamespace]]) bool {
		namespaces = append(namespaces, lk.Item().Read())
		return true
	})
	return namespaces
}

func (rm *ResourceManager) transformNamespace(ns *spec.Namespace) *spec.DGateNamespace {
	return &spec.DGateNamespace{
		Name: ns.Name,
		Tags: ns.Tags,
	}
}

func (rm *ResourceManager) AddNamespace(ns *spec.Namespace) *spec.DGateNamespace {
	defer rm.mutex.Lock(ns.Name)()
	namespace := rm.transformNamespace(ns)
	if nsLk, ok := rm.namespaces.Find(ns.Name); ok {
		nsLk.Item().Replace(namespace)
	} else {
		lk := linker.NewNamedVertexWithValue(
			safe.NewRef(namespace),
			"routes", "services",
			"modules", "domains",
			"collections", "secrets",
		)
		rm.namespaces.Insert(ns.Name, lk)
	}
	return namespace
}

func (rm *ResourceManager) RemoveNamespace(namespace string) error {
	defer rm.mutex.Lock(namespace)()
	if nsLk, ok := rm.namespaces.Find(namespace); ok {
		if nsLk.Len("routes") > 0 {
			return ErrCannotDeleteNamespace(namespace, "routes still linked")
		}
		if nsLk.Len("services") > 0 {
			return ErrCannotDeleteNamespace(namespace, "services still linked")
		}
		if nsLk.Len("modules") > 0 {
			return ErrCannotDeleteNamespace(namespace, "modules still linked")
		}
		if nsLk.Len("domains") > 0 {
			return ErrCannotDeleteNamespace(namespace, "domains still linked")
		}
		if nsLk.Len("collections") > 0 {
			return ErrCannotDeleteNamespace(namespace, "collections still linked")
		}
		if !rm.namespaces.Delete(namespace) {
			panic("failed to delete namespace")
		}
		return nil
	} else {
		return ErrNamespaceNotFound(namespace)
	}
}

/* Route functions */
func (rm *ResourceManager) GetRoute(name, namespace string) (*spec.DGateRoute, bool) {
	defer rm.mutex.RLock(namespace)()
	return rm.getRoute(name, namespace)
}

func (rm *ResourceManager) getRoute(name, namespace string) (*spec.DGateRoute, bool) {
	if lk, ok := rm.routes.Find(name + "/" + namespace); ok {
		return rm.updateRouteRef(lk), true
	}
	return nil, false
}

func (rm *ResourceManager) updateRouteRef(
	lk *linker.Link[string, safe.Ref[spec.DGateRoute]],
) *spec.DGateRoute {
	rt := lk.Item().Read()
	rt.Modules = []*spec.DGateModule{}
	lk.Each("modules", func(_ string, lk linker.Linker[string]) {
		mdLk := linker.NamedVertexWithVertex[string, safe.Ref[spec.DGateModule]](lk)
		rt.Modules = append(rt.Modules, mdLk.Item().Read())
	})
	if mods, ok := rm.getRouteModules(rt.Name, rt.Namespace.Name); ok {
		rt.Modules = mods
	}
	if rt.Service != nil {
		svcKey := rt.Service.Name + "/" + rt.Namespace.Name
		if svc, ok := rm.services.Find(svcKey); ok {
			rt.Service = svc.Item().Read()
		}
	}
	return rt
}

// GetRoutes returns a list of all routes
func (rm *ResourceManager) GetRoutes() []*spec.DGateRoute {
	defer rm.mutex.RLockMain()()
	var routes []*spec.DGateRoute
	rm.routes.Each(func(_ string, rtlk *linker.Link[string, safe.Ref[spec.DGateRoute]]) bool {
		routes = append(routes, rm.updateRouteRef(rtlk))
		return true
	})
	return routes
}

// GetRoutesByNamespace returns a list of all routes in a namespace
func (rm *ResourceManager) GetRoutesByNamespace(namespace string) []*spec.DGateRoute {
	defer rm.mutex.RLock(namespace)()
	var routes []*spec.DGateRoute
	if nsLk, ok := rm.namespaces.Find(namespace); ok {
		nsLk.Each("routes", func(_ string, lk linker.Linker[string]) {
			rtlk := linker.NamedVertexWithVertex[string, safe.Ref[spec.DGateRoute]](lk)
			routes = append(routes, rm.updateRouteRef(rtlk))
		})
	}
	return routes
}

// GetRouteNamespaceMap returns a map of all routes and their namespaces as the key
func (rm *ResourceManager) GetRouteNamespaceMap() map[string][]*spec.DGateRoute {
	defer rm.mutex.RLockMain()()
	routeMap := make(map[string][]*spec.DGateRoute)
	rm.namespaces.Each(func(ns string, lk *linker.Link[string, safe.Ref[spec.DGateNamespace]]) bool {
		routes := []*spec.DGateRoute{}
		lk.Each("routes", func(_ string, lk linker.Linker[string]) {
			rtlk := linker.NamedVertexWithVertex[string, safe.Ref[spec.DGateRoute]](lk)
			routes = append(routes, rm.updateRouteRef(rtlk))
		})
		if len(routes) > 0 {
			routeMap[ns] = routes
		}
		return true
	})
	return routeMap
}

func (rm *ResourceManager) AddRoute(route *spec.Route) (rt *spec.DGateRoute, err error) {
	defer rm.mutex.Lock(route.NamespaceName)()
	if rt, err = rm.transformRoute(route); err != nil {
		return nil, err
	} else if nsLk, ok := rm.namespaces.Find(route.NamespaceName); !ok {
		return nil, ErrNamespaceNotFound(route.NamespaceName)
	} else if rtLk, ok := rm.routes.Find(route.Name + "/" + route.NamespaceName); ok {
		rtLk.Item().Replace(rt)
		err = rm.relinkRoute(rtLk, nsLk, route, route.Name, route.NamespaceName, true)
		if err != nil {
			return nil, err
		}
		return rt, nil
	} else {
		rtLk := linker.NewNamedVertexWithValue(
			safe.NewRef(rt), "namespace", "service", "modules")
		err = rm.relinkRoute(rtLk, nsLk, route, route.Name, route.NamespaceName, false)
		if err != nil {
			return nil, err
		}
		rm.routes.Insert(route.Name+"/"+route.NamespaceName, rtLk)
		return rt, nil
	}
}

func (rm *ResourceManager) transformRoute(route *spec.Route) (*spec.DGateRoute, error) {
	if ns, ok := rm.getNamespace(route.NamespaceName); !ok {
		return nil, ErrNamespaceNotFound(route.NamespaceName)
	} else {
		var svc *spec.DGateService
		if route.ServiceName != "" {
			if svc, ok = rm.getService(route.ServiceName, route.NamespaceName); !ok {
				return nil, ErrServiceNotFound(route.ServiceName)
			}
		}
		mods := make([]*spec.DGateModule, len(route.Modules))
		for i, modName := range route.Modules {
			if mod, ok := rm.getModule(modName, route.NamespaceName); ok {
				mods[i] = mod
			} else {
				return nil, ErrModuleNotFound(modName)
			}
		}

		return &spec.DGateRoute{
			Name:         route.Name,
			Namespace:    ns,
			Paths:        route.Paths,
			Methods:      route.Methods,
			Service:      svc,
			Modules:      mods,
			StripPath:    route.StripPath,
			PreserveHost: route.PreserveHost,
			Tags:         route.Tags,
		}, nil
	}
}

// RemoveRoute removes a route from the resource manager
func (rm *ResourceManager) RemoveRoute(name, namespace string) error {
	defer rm.mutex.Lock(namespace)()
	if nsLk, ok := rm.namespaces.Find(namespace); !ok {
		return ErrNamespaceNotFound(namespace)
	} else if lk, ok := rm.routes.Find(name + "/" + namespace); ok {
		rm.unlinkRoute(lk, nsLk, name, namespace)
		if !rm.routes.Delete(name + "/" + namespace) {
			panic("failed to delete route")
		}
		return nil
	} else {
		return ErrRouteNotFound(name)
	}
}

func (rm *ResourceManager) unlinkRoute(
	rtLk *linker.Link[string, safe.Ref[spec.DGateRoute]],
	nsLk *linker.Link[string, safe.Ref[spec.DGateNamespace]],
	name, namespace string,
) {
	nsLk.UnlinkOneMany("routes", name)
	rtLk.UnlinkOneOneByKey("namespace", namespace)
	if svcLk := rtLk.Get("service"); svcLk != nil {
		svcLk.UnlinkOneMany("routes", name)
		rtLk.UnlinkOneOne("service")
	}
	rtLk.Each("modules", func(_ string, modLk linker.Linker[string]) {
		modLk.UnlinkOneMany("routes", name)
	})
}

func (rm *ResourceManager) relinkRoute(
	rtLk *linker.Link[string, safe.Ref[spec.DGateRoute]],
	nsLk *linker.Link[string, safe.Ref[spec.DGateNamespace]],
	route *spec.Route, name, namespace string, exists bool,
) error {
	modLks := make(map[string]linker.Linker[string], len(route.Modules))
	for _, modName := range route.Modules {
		if modLk, ok := rm.modules.Find(modName + "/" + route.NamespaceName); ok {
			modLks[modName] = modLk
		} else {
			return ErrModuleNotFound(modName)
		}
	}

	if route.ServiceName != "" {
		if svcLk, ok := rm.services.Find(route.ServiceName + "/" + route.NamespaceName); ok {
			if exists {
				rm.unlinkRoute(rtLk, nsLk, name, namespace)
			}
			rtLk.LinkOneOne("service", route.ServiceName, svcLk)
			svcLk.LinkOneMany("routes", route.Name, rtLk)
		} else {
			return ErrServiceNotFound(route.ServiceName)
		}
	}

	rtLk.LinkOneOne("namespace", route.NamespaceName, nsLk)
	nsLk.LinkOneMany("routes", route.Name, rtLk)

	for modName, modLk := range modLks {
		modLk.LinkOneMany("routes", route.Name, rtLk)
		rtLk.LinkOneMany("modules", modName, modLk)
	}
	return nil
}

/* Service functions */

func (rm *ResourceManager) GetService(name, namespace string) (*spec.DGateService, bool) {
	defer rm.mutex.RLock(namespace)()
	return rm.getService(name, namespace)
}

func (rm *ResourceManager) getService(name, namespace string) (*spec.DGateService, bool) {
	if lk, ok := rm.services.Find(name + "/" + namespace); ok {
		return lk.Item().Read(), true
	}
	return nil, false
}

// GetServicesByNamespace returns a list of all services in a namespace
func (rm *ResourceManager) GetServicesByNamespace(namespace string) []*spec.DGateService {
	defer rm.mutex.RLock(namespace)()
	var services []*spec.DGateService
	if nsLk, ok := rm.namespaces.Find(namespace); ok {
		nsLk.Each("services", func(_ string, lk linker.Linker[string]) {
			svcLk := linker.NamedVertexWithVertex[string, safe.Ref[spec.DGateService]](lk)
			services = append(services, svcLk.Item().Read())
		})
	}
	return services
}

// GetServices returns a list of all services
func (rm *ResourceManager) GetServices() []*spec.DGateService {
	defer rm.mutex.RLockMain()()
	var services []*spec.DGateService
	rm.services.Each(func(_ string, lk *linker.Link[string, safe.Ref[spec.DGateService]]) bool {
		services = append(services, lk.Item().Read())
		return true
	})
	return services
}

func (rm *ResourceManager) AddService(service *spec.Service) (*spec.DGateService, error) {
	defer rm.mutex.Lock(service.NamespaceName)()
	svc, err := rm.transformService(service)
	if err != nil {
		return nil, err
	}
	if svcLk, ok := rm.services.Find(service.Name + "/" + service.NamespaceName); ok {
		svcLk.Item().Replace(svc)
		return svc, nil
	} else {
		rw := safe.NewRef(svc)
		svcLk = linker.NewNamedVertexWithValue(rw, "routes", "namespaces")
		if nsLk, ok := rm.namespaces.Find(service.NamespaceName); ok {
			svcLk.LinkOneMany("namespaces", service.NamespaceName, nsLk)
			nsLk.LinkOneMany("services", service.Name, svcLk)
			rm.services.Insert(service.Name+"/"+service.NamespaceName, svcLk)
			return rw.Read(), nil
		} else {
			return nil, ErrNamespaceNotFound(service.NamespaceName)
		}
	}
}

func (rm *ResourceManager) transformService(service *spec.Service) (*spec.DGateService, error) {
	if ns, ok := rm.getNamespace(service.NamespaceName); !ok {
		return nil, ErrNamespaceNotFound(service.NamespaceName)
	} else {
		return spec.TransformService(ns, service), nil
	}
}

func (rm *ResourceManager) RemoveService(name, namespace string) error {
	defer rm.mutex.Lock(namespace)()
	if lk, ok := rm.services.Find(name + "/" + namespace); ok {
		if nsLk, ok := rm.namespaces.Find(namespace); ok {
			if rtsLk := lk.Get("routes"); rtsLk != nil {
				return ErrCannotDeleteService(name, "routes still linked")
			}
			nsLk.UnlinkOneMany("services", name)
			lk.UnlinkOneMany("namespaces", namespace)
		} else {
			return ErrNamespaceNotFound(namespace)
		}
		if !rm.services.Delete(name + "/" + namespace) {
			panic("failed to delete service")
		}
		return nil
	} else {
		return ErrServiceNotFound(name)
	}
}

/* Domain functions */

func (rm *ResourceManager) GetDomain(name, namespace string) (*spec.DGateDomain, bool) {
	defer rm.mutex.RLock(namespace)()
	return rm.getDomain(name, namespace)
}

func (rm *ResourceManager) getDomain(name, namespace string) (*spec.DGateDomain, bool) {
	if lk, ok := rm.domains.Find(name + "/" + namespace); ok {
		return lk.Item().Read(), true
	}
	return nil, false
}

// GetDomains returns a list of all domains
func (rm *ResourceManager) GetDomains() []*spec.DGateDomain {
	defer rm.mutex.RLockMain()()
	var domains []*spec.DGateDomain
	rm.domains.Each(func(_ string, lk *linker.Link[string, safe.Ref[spec.DGateDomain]]) bool {
		domains = append(domains, lk.Item().Read())
		return true
	})
	return domains
}

func (rm *ResourceManager) DomainCountEquals(target int) bool {
	if target < 0 {
		panic("target must be greater than or equal to 0")
	}
	defer rm.mutex.RLockMain()()
	rm.domains.Each(func(_ string, lk *linker.Link[string, safe.Ref[spec.DGateDomain]]) bool {
		target -= 1
		return target > 0
	})
	return target == 0
}

// GetDomainsByPriority returns a list of all domains sorted by priority and name
func (rm *ResourceManager) GetDomainsByPriority() []*spec.DGateDomain {
	defer rm.mutex.RLockMain()()
	if rm.priorityDomainCache != nil {
		return rm.priorityDomainCache
	}
	return rm.domainsByPriority()
}

func (rm *ResourceManager) domainsByPriority() []*spec.DGateDomain {
	var domains []*spec.DGateDomain
	rm.domains.Each(func(_ string, lk *linker.Link[string, safe.Ref[spec.DGateDomain]]) bool {
		domains = append(domains, lk.Item().Read())
		return true
	})
	sort.Slice(domains, func(i, j int) bool {
		d1, d2 := domains[j], domains[i]
		if d1.Priority == d2.Priority {
			return d1.Name < d2.Name
		}
		return d1.Priority < d2.Priority
	})
	rm.priorityDomainCache = domains
	return domains
}

// GetDomainsByNamespace returns a list of all domains in a namespace
func (rm *ResourceManager) GetDomainsByNamespace(namespace string) []*spec.DGateDomain {
	defer rm.mutex.RLock(namespace)()
	var domains []*spec.DGateDomain
	if nsLk, ok := rm.namespaces.Find(namespace); ok {
		nsLk.Each("domains", func(_ string, lk linker.Linker[string]) {
			dmLk := linker.NamedVertexWithVertex[string, safe.Ref[spec.DGateDomain]](lk)
			domains = append(domains, dmLk.Item().Read())
		})
	}
	return domains
}

// AddDomain adds a domain to the resource manager

func (rm *ResourceManager) AddDomain(domain *spec.Domain) (*spec.DGateDomain, error) {
	defer rm.mutex.Lock(domain.NamespaceName)()
	dm, err := rm.transformDomain(domain)
	if err != nil {
		return nil, err
	}
	defer func() { rm.priorityDomainCache = nil }()
	if dmLk, ok := rm.domains.Find(domain.Name + "/" + domain.NamespaceName); ok {
		dmLk.Item().Replace(dm)
		return dm, nil
	} else {
		rw := safe.NewRef(dm)
		dmLk = linker.NewNamedVertexWithValue(rw, "namespace")
		if nsLk, ok := rm.namespaces.Find(domain.NamespaceName); ok {
			nsLk.LinkOneMany("domains", domain.Name, dmLk)
			dmLk.LinkOneOne("namespace", domain.NamespaceName, nsLk)
			rm.domains.Insert(domain.Name+"/"+domain.NamespaceName, dmLk)
			return rw.Read(), nil
		}
	}
	return nil, ErrNamespaceNotFound(domain.NamespaceName)
}

func (rm *ResourceManager) transformDomain(domain *spec.Domain) (*spec.DGateDomain, error) {
	if ns, ok := rm.getNamespace(domain.NamespaceName); !ok {
		return nil, ErrNamespaceNotFound(domain.NamespaceName)
	} else {
		return &spec.DGateDomain{
			Name:      domain.Name,
			Namespace: ns,
			Patterns:  domain.Patterns,
			Priority:  domain.Priority,
			Cert:      domain.Cert,
			Key:       domain.Key,
			Tags:      domain.Tags,
		}, nil
	}
}

func (rm *ResourceManager) RemoveDomain(name, namespace string) error {
	defer rm.mutex.Lock(namespace)()
	defer func() { rm.priorityDomainCache = nil }()
	if dmLk, ok := rm.domains.Find(name + "/" + namespace); ok {
		if nsLk, ok := rm.namespaces.Find(namespace); ok {
			nsLk.UnlinkOneMany("domains", name)
			dmLk.UnlinkOneOneByKey("namespace", namespace)
			if !rm.domains.Delete(name + "/" + namespace) {
				panic("failed to delete domain")
			}
		} else {
			return ErrNamespaceNotFound(namespace)
		}
	} else {
		return ErrDomainNotFound(name)
	}
	return nil
}

/* Module functions */

func (rm *ResourceManager) GetModule(name, namespace string) (*spec.DGateModule, bool) {
	defer rm.mutex.RLock(namespace)()
	return rm.getModule(name, namespace)
}

func (rm *ResourceManager) getModule(name, namespace string) (*spec.DGateModule, bool) {
	if lk, ok := rm.modules.Find(name + "/" + namespace); ok {
		return lk.Item().Read(), true
	}
	return nil, false
}

// GetModules returns a list of all modules
func (rm *ResourceManager) GetModules() []*spec.DGateModule {
	defer rm.mutex.RLockMain()()
	var modules []*spec.DGateModule
	rm.modules.Each(func(_ string, lk *linker.Link[string, safe.Ref[spec.DGateModule]]) bool {
		modules = append(modules, lk.Item().Read())
		return true
	})
	return modules
}

// GetRouteModules returns a list of all modules in a route
func (rm *ResourceManager) GetRouteModules(name, namespace string) ([]*spec.DGateModule, bool) {
	defer rm.mutex.RLock(namespace)()
	return rm.getRouteModules(name, namespace)
}

// getRouteModules returns a list of all modules in a route
func (rm *ResourceManager) getRouteModules(name, namespace string) ([]*spec.DGateModule, bool) {
	if lk, ok := rm.routes.Find(name + "/" + namespace); ok {
		rt := lk.Item().Read()
		var modules []*spec.DGateModule
		if rtLk, ok := rm.routes.Find(rt.Name + "/" + rt.Namespace.Name); ok {
			rtLk.Each("modules", func(_ string, lk linker.Linker[string]) {
				mdLk := linker.NamedVertexWithVertex[string, safe.Ref[spec.DGateModule]](lk)
				modules = append(modules, mdLk.Item().Read())
			})
		}
		return modules, true
	} else {
		return nil, false
	}
}

// GetModulesByNamespace returns a list of all modules in a namespace
func (rm *ResourceManager) GetModulesByNamespace(namespace string) []*spec.DGateModule {
	defer rm.mutex.RLock(namespace)()
	var modules []*spec.DGateModule
	if nsLk, ok := rm.namespaces.Find(namespace); ok {
		nsLk.Each("modules", func(_ string, lk linker.Linker[string]) {
			mdLk := linker.NamedVertexWithVertex[string, safe.Ref[spec.DGateModule]](lk)
			modules = append(modules, mdLk.Item().Read())
		})
	}
	return modules
}

func (rm *ResourceManager) AddModule(module *spec.Module) (*spec.DGateModule, error) {
	defer rm.mutex.Lock(module.NamespaceName)()
	if md, err := rm.transformModule(module); err != nil {
		return nil, err
	} else if modLk, ok := rm.modules.Find(module.Name + "/" + module.NamespaceName); ok {
		modLk.Item().Replace(md)
		return md, nil
	} else {
		rw := safe.NewRef(md)
		modLk = linker.NewNamedVertexWithValue(rw, "namespace", "routes")
		if nsLk, ok := rm.namespaces.Find(module.NamespaceName); ok {
			nsLk.LinkOneMany("modules", module.Name, modLk)
			modLk.LinkOneOne("namespace", module.NamespaceName, nsLk)
			rm.modules.Insert(module.Name+"/"+module.NamespaceName, modLk)
			return rw.Read(), nil
		} else {
			return nil, ErrNamespaceNotFound(module.NamespaceName)
		}
	}
}

func (rm *ResourceManager) transformModule(module *spec.Module) (*spec.DGateModule, error) {
	if ns, ok := rm.getNamespace(module.NamespaceName); !ok {
		return nil, ErrNamespaceNotFound(module.NamespaceName)
	} else {
		return spec.TransformModule(ns, module)
	}
}

func (rm *ResourceManager) RemoveModule(name, namespace string) error {
	defer rm.mutex.Lock(namespace)()
	if modLink, ok := rm.modules.Find(name + "/" + namespace); ok {
		if modLink.Len("routes") > 0 {
			return ErrCannotDeleteModule(name, "routes still linked")
		}
		if nsLk, ok := rm.namespaces.Find(namespace); !ok {
			return ErrNamespaceNotFound(namespace)
		} else {
			nsLk.UnlinkOneMany("modules", name)
			modLink.UnlinkOneOne("namespace")
		}
		if !rm.modules.Delete(name + "/" + namespace) {
			panic("failed to delete module")
		}
		return nil
	} else {
		return ErrModuleNotFound(name)
	}
}

/* Collection functions */

func (rm *ResourceManager) GetCollection(name, namespace string) (*spec.DGateCollection, bool) {
	defer rm.mutex.RLock(namespace)()
	return getCollection(rm, name, namespace)
}

func getCollection(rm *ResourceManager, name, namespace string) (*spec.DGateCollection, bool) {
	if lk, ok := rm.collections.Find(name + "/" + namespace); ok {
		return lk.Item().Read(), true
	}
	return nil, false
}

func (rm *ResourceManager) GetCollectionsByNamespace(namespace string) []*spec.DGateCollection {
	defer rm.mutex.RLock(namespace)()
	var collections []*spec.DGateCollection
	if nsLk, ok := rm.namespaces.Find(namespace); ok {
		nsLk.Each("collections", func(_ string, lk linker.Linker[string]) {
			clLk := linker.NamedVertexWithVertex[string, safe.Ref[spec.DGateCollection]](lk)
			collections = append(collections, clLk.Item().Read())
		})
	}
	return collections
}

// GetCollections returns a list of all collections
func (rm *ResourceManager) GetCollections() []*spec.DGateCollection {
	defer rm.mutex.RLockMain()()
	var collections []*spec.DGateCollection
	rm.collections.Each(func(_ string, lk *linker.Link[string, safe.Ref[spec.DGateCollection]]) bool {
		collections = append(collections, lk.Item().Read())
		return true
	})
	return collections
}

func (rm *ResourceManager) AddCollection(collection *spec.Collection) (*spec.DGateCollection, error) {
	defer rm.mutex.Lock(collection.NamespaceName)()
	cl, err := rm.transformCollection(collection)
	if err != nil {
		return nil, err
	}
	if clLk, ok := rm.collections.Find(collection.Name + "/" + collection.NamespaceName); ok {
		clLk.Item().Replace(cl)
		return cl, nil
	} else {
		rw := safe.NewRef(cl)
		colLk := linker.NewNamedVertexWithValue(rw, "namespace")
		if nsLk, ok := rm.namespaces.Find(collection.NamespaceName); ok {
			nsLk.LinkOneMany("collections", collection.Name, colLk)
			colLk.LinkOneOne("namespace", collection.NamespaceName, nsLk)
			rm.collections.Insert(collection.Name+"/"+collection.NamespaceName, colLk)
			return rw.Read(), nil
		} else {
			return nil, ErrNamespaceNotFound(collection.NamespaceName)
		}
	}
}

func (rm *ResourceManager) transformCollection(collection *spec.Collection) (*spec.DGateCollection, error) {
	if ns, ok := rm.getNamespace(collection.NamespaceName); !ok {
		return nil, ErrNamespaceNotFound(collection.NamespaceName)
	} else {
		// if mods, err := sliceutil.SliceMapperError(collection.Modules, func(modName string) (*spec.DGateModule, error) {
		// 	if mod, ok := rm.getModule(modName, collection.NamespaceName); ok {
		// 		return mod, nil
		// 	}
		// 	return nil, ErrModuleNotFound(collection.NamespaceName)
		// }); err != nil {
		// 	return nil, err
		// } else {
		// 	return spec.TransformCollection(ns, mods, collection), nil
		// }
		return spec.TransformCollection(ns, nil, collection), nil
	}
}

func (rm *ResourceManager) RemoveCollection(name, namespace string) error {
	defer rm.mutex.Lock(namespace)()
	if colLk, ok := rm.collections.Find(name + "/" + namespace); ok {
		if nsLk, ok := rm.namespaces.Find(namespace); ok {
			// unlink namespace to collection
			nsLk.UnlinkOneMany("collections", name)
			// unlink collection to namespace
			colLk.UnlinkOneOne("namespace")
			if !rm.collections.Delete(name + "/" + namespace) {
				panic("failed to delete collection")
			}
		} else {
			return ErrNamespaceNotFound(namespace)
		}
	} else {
		return ErrCollectionNotFound(name)
	}
	return nil
}

/* Secret functions */

func (rm *ResourceManager) GetSecret(name, namespace string) (*spec.DGateSecret, bool) {
	defer rm.mutex.RLock(namespace)()
	return rm.getSecret(name, namespace)
}

func (rm *ResourceManager) getSecret(name, namespace string) (*spec.DGateSecret, bool) {
	if lk, ok := rm.secrets.Find(name + "/" + namespace); ok {
		return lk.Item().Read(), true
	}
	return nil, false
}

// GetSecrets returns a list of all secrets
func (rm *ResourceManager) GetSecrets() []*spec.DGateSecret {
	defer rm.mutex.RLockMain()()
	var secrets []*spec.DGateSecret
	rm.secrets.Each(func(_ string, lk *linker.Link[string, safe.Ref[spec.DGateSecret]]) bool {
		secrets = append(secrets, lk.Item().Read())
		return true
	})
	return secrets
}

// GetSecretsByNamespace returns a list of all secrets in a namespace
func (rm *ResourceManager) GetSecretsByNamespace(namespace string) []*spec.DGateSecret {
	defer rm.mutex.RLock(namespace)()
	var secrets []*spec.DGateSecret
	if nsLk, ok := rm.namespaces.Find(namespace); ok {
		nsLk.Each("secrets", func(_ string, lk linker.Linker[string]) {
			mdLk := linker.NamedVertexWithVertex[string, safe.Ref[spec.DGateSecret]](lk)
			secrets = append(secrets, mdLk.Item().Read())
		})
	}
	return secrets
}

func (rm *ResourceManager) AddSecret(secret *spec.Secret) (*spec.DGateSecret, error) {
	defer rm.mutex.Lock(secret.NamespaceName)()
	sec, err := rm.transformSecret(secret)
	if err != nil {
		return nil, err
	}
	if rw, ok := rm.secrets.Find(secret.Name + "/" + secret.NamespaceName); ok {
		rw.Item().Replace(sec)
		return sec, nil
	} else {
		rw := safe.NewRef(sec)
		scrtLk := linker.NewNamedVertexWithValue(rw, "namespace")
		if nsLk, ok := rm.namespaces.Find(secret.NamespaceName); ok {
			nsLk.LinkOneMany("secrets", secret.Name, scrtLk)
			scrtLk.LinkOneOne("namespace", secret.NamespaceName, nsLk)
			rm.secrets.Insert(secret.Name+"/"+secret.NamespaceName, scrtLk)
			return rw.Read(), nil
		} else {
			return nil, ErrNamespaceNotFound(secret.NamespaceName)
		}
	}
}

func (rm *ResourceManager) transformSecret(secret *spec.Secret) (*spec.DGateSecret, error) {
	if ns, ok := rm.getNamespace(secret.NamespaceName); !ok {
		return nil, ErrNamespaceNotFound(secret.NamespaceName)
	} else {
		return spec.TransformSecret(ns, secret)
	}
}

func (rm *ResourceManager) RemoveSecret(name, namespace string) error {
	defer rm.mutex.Lock(namespace)()
	if scrtLink, ok := rm.secrets.Find(name + "/" + namespace); ok {
		if scrtLink.Len("routes") > 0 {
			return ErrCannotDeleteSecret(name, "routes still linked")
		}
		if nsLk, ok := rm.namespaces.Find(namespace); !ok {
			return ErrNamespaceNotFound(namespace)
		} else {
			nsLk.UnlinkOneMany("secrets", name)
			scrtLink.UnlinkOneOne("namespace")
		}
		if !rm.secrets.Delete(name + "/" + namespace) {
			panic("failed to delete secret")
		}
		return nil
	} else {
		return ErrSecretNotFound(name)
	}
}

// Clear removes all resources from the resource manager
func (rm *ResourceManager) Clear() {
	defer rm.mutex.LockMain()()
	rm.namespaces.Clear()
	rm.services.Clear()
	rm.domains.Clear()
	rm.modules.Clear()
	rm.routes.Clear()
	rm.collections.Clear()
	rm.secrets.Clear()
}

// Empty returns true if the resource manager is empty
func (rm *ResourceManager) Empty() bool {
	defer rm.mutex.RLockMain()()
	return rm.namespaces.Empty() &&
		rm.services.Empty() &&
		rm.domains.Empty() &&
		rm.modules.Empty() &&
		rm.routes.Empty() &&
		rm.collections.Empty()

}

func ErrCollectionNotFound(name string) error {
	return errors.New("collection not found: " + name)
}

func ErrDomainNotFound(name string) error {
	return errors.New("domain not found: " + name)
}

func ErrNamespaceNotFound(name string) error {
	return errors.New("namespace not found: " + name)
}

func ErrServiceNotFound(name string) error {
	return errors.New("service not found: " + name)
}

func ErrModuleNotFound(name string) error {
	return errors.New("module not found: " + name)
}

func ErrRouteNotFound(name string) error {
	return errors.New("route not found: " + name)
}

func ErrSecretNotFound(name string) error {
	return errors.New("secret not found: " + name)
}

func ErrCannotDeleteModule(name, reason string) error {
	return errors.New("cannot delete module: " + name + ": " + reason)
}

func ErrCannotDeleteService(name, reason string) error {
	return errors.New("cannot delete service: " + name + ": " + reason)
}

func ErrCannotDeleteNamespace(name, reason string) error {
	return errors.New("cannot delete namespace: " + name + ": " + reason)
}

func ErrCannotDeleteRoute(name, reason string) error {
	return errors.New("cannot delete route: " + name + ": " + reason)
}

func ErrCannotDeleteDomain(name, reason string) error {
	return errors.New("cannot delete domain: " + name + ": " + reason)
}

func ErrCannotDeleteCollection(name, reason string) error {
	return errors.New("cannot delete collection: " + name + ": " + reason)
}

func ErrCannotDeleteSecret(name, reason string) error {
	return errors.New("cannot delete secret: " + name + ": " + reason)
}
