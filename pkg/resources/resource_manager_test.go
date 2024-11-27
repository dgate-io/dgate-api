package resources_test

import (
	"encoding/base64"
	"strconv"
	"testing"

	"github.com/dgate-io/dgate-api/pkg/resources"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/stretchr/testify/assert"
)

func TestResourceManager(t *testing.T) {
	rm := resources.NewManager()

	rm.AddNamespace(&spec.Namespace{
		Name: "test",
		Tags: []string{"test"},
	})

	_, err := rm.AddService(&spec.Service{
		Name: "test",
		URLs: []string{
			"http://localhost:8080",
		},
		NamespaceName: "test",
		Tags:          []string{"test"},
	})
	assert.True(t, err == nil, err)

	_, err = rm.AddModule(&spec.Module{
		Name: "test",
		Payload: base64.StdEncoding.EncodeToString(
			[]byte(`export const onRequest = (ctx, req, res) => {
			res.send('Hello, World!');
		}`)),
		NamespaceName: "test",
		Tags:          []string{"test"},
	})
	assert.True(t, err == nil, err)

	routeCount := 10
	for i := 0; i < routeCount; i++ {
		_, err = rm.AddRoute(&spec.Route{
			Name:          "test" + strconv.Itoa(i),
			Paths:         []string{"/", "/test"},
			Methods:       []string{"GET", "PUT"},
			Modules:       []string{"test"},
			ServiceName:   "test",
			NamespaceName: "test",
			Tags:          []string{"test"},
		})
		assert.True(t, err == nil, err)
	}

	{
		routes := rm.GetRoutesByNamespace("test")
		if len(routes) != routeCount {
			t.Errorf("Expected 1 route, got %d", len(routes))
		}
		for i, route := range routes {
			if route.Name != "test"+strconv.Itoa(i) {
				t.Errorf("Expected route name 'test%d', got %s", i, route.Name)
			}
		}
	}

	{
		routeMap := rm.GetRouteNamespaceMap()
		if len(routeMap) != 1 {
			t.Errorf("Expected 1 route namespace, got %d", len(routeMap))
		}
		routes, ok := routeMap["test"]
		if !ok {
			t.Error("Expected route namespace 'test', got none")
		}
		if len(routes) != routeCount {
			t.Errorf("Expected 1 route, got %d", len(routes))
		}
		for i, route := range routes {
			if route.Name != "test"+strconv.Itoa(i) {
				t.Errorf("Expected route name 'test%d', got %s", i, route.Name)
			}
		}
	}
}

func TestResourceManagerNamespaceScope(t *testing.T) {
	rm := resources.NewManager()

	rm.AddNamespace(&spec.Namespace{
		Name: "test1",
		Tags: []string{"test"},
	})
	rm.AddNamespace(&spec.Namespace{
		Name: "test2",
		Tags: []string{"test"},
	})

	_, err := rm.AddService(&spec.Service{
		Name:          "test2",
		URLs:          []string{"http://localhost:8080"},
		NamespaceName: "test2",
		Tags:          []string{"test"},
	})
	assert.True(t, err == nil, err)

	_, err = rm.AddRoute(&spec.Route{
		Name:          "test1",
		Paths:         []string{"/", "/test"},
		Methods:       []string{"GET", "PUT"},
		ServiceName:   "test2",
		NamespaceName: "test1",
		Tags:          []string{"test"},
	})
	assert.EqualError(t, err, resources.ErrServiceNotFound("test2").Error())

	if routes := rm.GetRoutesByNamespace("test"); len(routes) != 0 {
		t.Errorf("Expected 1 route, got %d", len(routes))
	}

	if routeMap := rm.GetRouteNamespaceMap(); len(routeMap) != 0 {
		t.Errorf("Expected 1 route namespace, got %d", len(routeMap))
	}
}

func makeCommonResources(
	t *testing.T,
	rm *resources.ResourceManager,
	nsid, id string,
	deleteNamespace bool,
) func() {
	t.Logf("Creating resources with nsid: %s, id: %s", nsid, id)
	ns := rm.AddNamespace(&spec.Namespace{
		Name: "test_ns" + nsid,
		Tags: []string{"test", nsid},
	})

	svc, err := rm.AddService(&spec.Service{
		Name:          "test_svc" + id,
		URLs:          []string{"http://localhost:8080"},
		NamespaceName: ns.Name,
		Tags:          []string{"test", id},
	})
	if !assert.True(t, err == nil, err) {
		t.FailNow()
	}

	mod, err := rm.AddModule(&spec.Module{
		Name: "test_mod" + id,
		Payload: base64.StdEncoding.EncodeToString(
			[]byte(`export const onRequest = (ctx, req, res) => {
			res.send('Hello, World!');
		}`)),
		NamespaceName: ns.Name,
		Tags:          []string{"test", id},
	})
	if !assert.True(t, err == nil, err) {
		t.FailNow()
	}

	rt, err := rm.AddRoute(&spec.Route{
		Name:          "test_rt" + id,
		Paths:         []string{"/"},
		Methods:       []string{"GET"},
		ServiceName:   svc.Name,
		Modules:       []string{mod.Name},
		NamespaceName: ns.Name,
		Tags:          []string{"test", id},
	})
	if !assert.True(t, err == nil, err) {
		t.FailNow()
	}

	dm, err := rm.AddDomain(&spec.Domain{
		Name:          "test_dm" + id,
		NamespaceName: ns.Name,
		Tags:          []string{"test", id},
	})
	if !assert.True(t, err == nil, err) {
		t.FailNow()
	}

	col, err := rm.AddCollection(&spec.Collection{
		Name:          "test_col" + id,
		NamespaceName: ns.Name,
		Tags:          []string{"test", id},
	})
	if !assert.True(t, err == nil, err) {
		t.FailNow()
	}

	return func() {
		t.Logf("Removing resources with nsid: %s, id: %s", nsid, id)
		err = rm.RemoveModule(mod.Name, ns.Name)
		if assert.NotNil(t, err) {
			expErr := resources.ErrCannotDeleteModule(mod.Name, "routes still linked")
			assert.EqualError(t, expErr, err.Error())
		} else {
			t.FailNow()
		}

		err = rm.RemoveService(svc.Name, ns.Name)
		if assert.NotNil(t, err) {
			expErr := resources.ErrCannotDeleteService(svc.Name, "routes still linked")
			assert.EqualError(t, err, expErr.Error())
		} else {
			t.FailNow()
		}

		if deleteNamespace {
			err = rm.RemoveNamespace(ns.Name)
			if assert.NotNil(t, err) {
				expErr := resources.ErrCannotDeleteNamespace(ns.Name, "routes still linked")
				assert.EqualError(t, err, expErr.Error())

			} else {
				t.FailNow()
			}
		}

		err = rm.RemoveRoute(rt.Name, ns.Name)
		if assert.Nil(t, err) {
			if deleteNamespace {
				err = rm.RemoveNamespace(ns.Name)
				assert.EqualError(t, err, resources.ErrCannotDeleteNamespace(ns.Name, "services still linked").Error())
			}
		} else {
			t.FailNow()
		}

		err = rm.RemoveService(svc.Name, ns.Name)
		if assert.Nil(t, err) {
			if deleteNamespace {
				err = rm.RemoveNamespace(ns.Name)
				assert.EqualError(t, err, resources.ErrCannotDeleteNamespace(ns.Name, "modules still linked").Error())
			}
		} else {
			t.FailNow()
		}

		err = rm.RemoveModule(mod.Name, ns.Name)
		if assert.Nil(t, err) {
			if deleteNamespace {
				err = rm.RemoveNamespace(ns.Name)
				assert.EqualError(t, err, resources.ErrCannotDeleteNamespace(ns.Name, "domains still linked").Error())
			}
		} else {
			t.FailNow()
		}

		err = rm.RemoveDomain(dm.Name, ns.Name)
		if assert.Nil(t, err) {
			if deleteNamespace {
				err = rm.RemoveNamespace(ns.Name)
				assert.EqualError(t, err, resources.ErrCannotDeleteNamespace(ns.Name, "collections still linked").Error())
			}
		} else {
			t.FailNow()
		}

		err = rm.RemoveCollection(col.Name, ns.Name)
		if assert.Nil(t, err) {
			if deleteNamespace {
				err = rm.RemoveNamespace(ns.Name)
				assert.Nil(t, err)
			}
		} else {
			t.FailNow()
		}
	}
}

func TestResourceManagerDependency_ForwardNamespaceClear(t *testing.T) {
	rm := resources.NewManager()
	makeCommonResources(t, rm, "1", "1", true)()
	makeCommonResources(t, rm, "2", "1", true)()
	makeCommonResources(t, rm, "3", "1", true)()
	makeCommonResources(t, rm, "4", "1", true)()
	makeCommonResources(t, rm, "5", "1", true)()
	assert.True(t, rm.Empty(), "expected resources to be empty")
}

func TestResourceManagerDependency_ForwardResourceClear(t *testing.T) {
	rm := resources.NewManager()
	func() {
		defer makeCommonResources(t, rm, "1", "1", true)()
		defer makeCommonResources(t, rm, "1", "2", false)()
		defer makeCommonResources(t, rm, "1", "3", false)()
		defer makeCommonResources(t, rm, "1", "4", false)()
		defer makeCommonResources(t, rm, "1", "5", false)()
	}()
	assert.True(t, rm.Empty(), "expected resources to be empty")
}

func TestResourceManagerDependency_BackwardsNamespaceClear(t *testing.T) {
	rm := resources.NewManager()
	func() {
		defer makeCommonResources(t, rm, "1", "1", true)()
		defer makeCommonResources(t, rm, "2", "1", true)()
		defer makeCommonResources(t, rm, "3", "1", true)()
		defer makeCommonResources(t, rm, "4", "1", true)()
		defer makeCommonResources(t, rm, "5", "1", true)()
	}()
	assert.True(t, rm.Empty(), "expected resources to be empty")
}

func TestResourceManagerDependency_BackwardsResourceClear(t *testing.T) { // flawed test: won't be able to delete the resource because of deps
	rm := resources.NewManager()
	func() {
		defer makeCommonResources(t, rm, "1", "1", true)()
		defer makeCommonResources(t, rm, "1", "2", false)()
		defer makeCommonResources(t, rm, "1", "3", false)()
		defer makeCommonResources(t, rm, "1", "4", false)()
		defer makeCommonResources(t, rm, "1", "5", false)()
	}()
	assert.True(t, rm.Empty(), "expected resources to be empty")
}

// TODO: Add dependency test to ensure child/parent
func TestResourceManagerDependency_(t *testing.T) {
	rm := resources.NewManager()

	func() {
		defer makeCommonResources(t, rm, "1", "1", true)()
		defer makeCommonResources(t, rm, "1", "2", false)()

		_, ok := rm.GetRouteModules("test_rt1", "test_ns1")
		assert.True(t, ok, "expected route to exist")

		routes := rm.GetRoutesByNamespace("test_ns1")
		if assert.True(t, ok) {
			assert.Equal(t, 2, len(routes))
		}

		services := rm.GetServicesByNamespace("test_ns1")
		if assert.True(t, ok) {
			assert.Equal(t, 2, len(services))
		}

		modules := rm.GetModulesByNamespace("test_ns1")
		if assert.True(t, ok) {
			assert.Equal(t, 2, len(modules))
		}

		collections := rm.GetCollectionsByNamespace("test_ns1")
		if assert.True(t, ok) {
			assert.Equal(t, 2, len(collections))
		}

		domains := rm.GetDomainsByNamespace("test_ns1")
		if assert.True(t, ok) {
			assert.Equal(t, 2, len(domains))
		}
	}()
	assert.True(t, rm.Empty(), "expected resources to be empty")
}

func TestResourceManagerDependency__(t *testing.T) {
	rm := resources.NewManager()

	func() {
		defer makeCommonResources(t, rm, "1", "1", true)()
		defer makeCommonResources(t, rm, "1", "2", false)()

		_, ok := rm.GetRouteModules("test_rt1", "test_ns1")
		assert.True(t, ok, "expected route to exist")

		routeMap := rm.GetRouteNamespaceMap()
		if assert.True(t, ok) {
			assert.Equal(t, 1, len(routeMap))
			assert.Equal(t, 2, len(routeMap["test_ns1"]))
		}

		if mod, ok := rm.GetRouteModules("test_rt1", "test_ns1"); assert.True(t, ok) {
			assert.Equal(t, 1, len(mod))
		}

		if mod, ok := rm.GetRouteModules("test_rt2", "test_ns1"); assert.True(t, ok) {
			assert.Equal(t, 1, len(mod))
		}
	}()
	assert.True(t, rm.Empty(), "expected resources to be empty")
}

// TODO: AtomicityTest to check that the state hasn't changed, in the event of an error.

func BenchmarkRM_ParallelRouteReading(b *testing.B) {
	rm := resources.NewManager()

	rm.AddNamespace(&spec.Namespace{
		Name: "test",
		Tags: []string{"test"},
	})

	_, err := rm.AddService(&spec.Service{
		Name:          "test",
		URLs:          []string{"http://localhost:8080"},
		NamespaceName: "test",
		Tags:          []string{"test"},
	})
	assert.True(b, err == nil, err)

	_, err = rm.AddModule(&spec.Module{
		Name: "test",
		Payload: base64.StdEncoding.EncodeToString(
			[]byte(`export const onRequest = (ctx, req, res) => {
			res.send('Hello, World!');
		}`)),
		NamespaceName: "test",
		Tags:          []string{"test"},
	})
	assert.True(b, err == nil, err)

	rt, err := rm.AddRoute(&spec.Route{
		Name:          "test1",
		Paths:         []string{"/", "/test"},
		Methods:       []string{"GET", "PUT"},
		Modules:       []string{"test"},
		ServiceName:   "test",
		NamespaceName: "test",
		Tags:          []string{"test"},
	})
	assert.True(b, err == nil, err)
	b.SetParallelism(10)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			route, ok := rm.GetRoute("test1", "test")
			if assert.True(b, ok) {
				assert.Equal(b, rt, route)
			}
		}
	})
}

func BenchmarkRM_ParallelReadWrite(b *testing.B) {
	rm := resources.NewManager()

	rm.AddNamespace(&spec.Namespace{
		Name: "test",
		Tags: []string{"test"},
	})

	_, err := rm.AddService(&spec.Service{
		Name:          "test",
		URLs:          []string{"http://localhost:8080"},
		NamespaceName: "test",
		Tags:          []string{"test"},
	})
	if !assert.True(b, err == nil, err) {
		b.FailNow()
	}

	_, err = rm.AddModule(&spec.Module{
		Name: "test",
		Payload: base64.StdEncoding.EncodeToString(
			[]byte(`export const onRequest = (ctx, req, res) => {
			res.send('Hello, World!');
		}`)),
		NamespaceName: "test",
		Tags:          []string{"test"},
	})
	if !assert.True(b, err == nil, err) {
		b.FailNow()
	}

	_, err = rm.AddRoute(&spec.Route{
		Name:          "test1",
		Paths:         []string{"/"},
		Methods:       []string{"GET"},
		Modules:       []string{"test"},
		ServiceName:   "test",
		NamespaceName: "test",
	})
	if !assert.True(b, err == nil, err) {
		b.FailNow()
	}

	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			_, err := rm.AddRoute(&spec.Route{
				Name:          "test1",
				Paths:         []string{"/"},
				Methods:       []string{"GET"},
				Modules:       []string{"test"},
				ServiceName:   "test",
				NamespaceName: "test",
			})
			if err != nil {
				b.FailNow()
			}
		}
	})

	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			if _, ok := rm.GetRoute("test1", "test"); !ok {
				b.FailNow()
			}
		}
	})
}

func BenchmarkRM_ReadingWriting(b *testing.B) {
	rm := resources.NewManager()
	routeSize := 100
	modSvcSize := 10
	nsSize := 10

	for i := 0; i < nsSize; i++ {
		nsName := "test" + strconv.Itoa(i)
		rm.AddNamespace(&spec.Namespace{
			Name: nsName,
			Tags: []string{"test"},
		})
		for j := 0; j < modSvcSize; j++ {
			svcModName := "test" + strconv.Itoa(j)
			_, err := rm.AddService(&spec.Service{
				Name:          svcModName,
				URLs:          []string{"http://localhost:8080"},
				NamespaceName: nsName,
				Tags:          []string{"test"},
			})
			assert.Nil(b, err)

			_, err = rm.AddModule(&spec.Module{
				Name: svcModName,
				Payload: base64.StdEncoding.EncodeToString(
					[]byte(``)),
				NamespaceName: nsName,
				Tags:          []string{"test"},
			})
			assert.Nil(b, err)

			for k := 0; k < routeSize; k++ {
				_, err = rm.AddRoute(&spec.Route{
					Name:          "test" + strconv.Itoa(j*modSvcSize+k),
					Paths:         []string{"/", "/test"},
					Methods:       []string{"GET", "PUT"},
					Modules:       []string{svcModName},
					ServiceName:   svcModName,
					NamespaceName: nsName,
					Tags:          []string{"test"},
				})
				assert.Nil(b, err)
			}
		}
	}
	b.ResetTimer()

	b.Run("GetRoute", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for i := 0; pb.Next(); i++ {
				_, ok := rm.GetRoute(
					"test"+strconv.Itoa(i%routeSize),
					"test"+strconv.Itoa(i%nsSize),
				)
				if !ok {
					b.FailNow()
				}
			}
		})
	})

	b.Run("GetRouteModules", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for i := 0; pb.Next(); i++ {
				_, ok := rm.GetRouteModules(
					"test"+strconv.Itoa(i%routeSize),
					"test"+strconv.Itoa(i%nsSize),
				)
				if !assert.True(b, ok) {
					b.FailNow()
				}
			}
		})
	})

	b.Run("AddRoute", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			rt := &spec.Route{
				Paths:   []string{"/"},
				Methods: []string{"GET"},
				Modules: []string{""},
				Tags:    []string{"test"},
			}
			for i := 0; pb.Next(); i++ {
				rt.Name = "test" + strconv.Itoa(i%routeSize)
				rt.Modules[0] = "test" + strconv.Itoa(i%modSvcSize)
				rt.ServiceName = "test" + strconv.Itoa(i%modSvcSize)
				rt.NamespaceName = "test" + strconv.Itoa(i%nsSize)
				_, err := rm.AddRoute(rt)
				if !assert.True(b, err == nil, err) {
					b.FailNow()
				}
			}
		})
	})

	b.Run("AddModule", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for i := 0; pb.Next(); i++ {
				_, err := rm.AddModule(&spec.Module{
					Name:          "test" + strconv.Itoa(i%modSvcSize),
					Payload:       base64.StdEncoding.EncodeToString([]byte(``)),
					NamespaceName: "test" + strconv.Itoa(i%nsSize),
					Tags:          []string{"test"},
				})
				if !assert.True(b, err == nil, err) {
					b.FailNow()
				}
			}
		})
	})

	b.Run("AddService", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for i := 0; pb.Next(); i++ {
				_, err := rm.AddService(&spec.Service{
					Name:          "test" + strconv.Itoa(i%modSvcSize),
					NamespaceName: "test" + strconv.Itoa(i%nsSize),
					URLs:          []string{"http://localhost:8080"},
					Tags:          []string{"test"},
				})
				if !assert.True(b, err == nil, err) {
					b.FailNow()
				}
			}
		})
	})

	b.Run("AddNamespace", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for i := 0; pb.Next(); i++ {
				ns := rm.AddNamespace(&spec.Namespace{
					Name: "test" + strconv.Itoa(i%100),
					Tags: []string{"test"},
				})
				if !assert.True(b, ns != nil) {
					b.FailNow()
				}
			}
		})
	})
}
