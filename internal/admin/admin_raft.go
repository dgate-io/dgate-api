package admin

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/http"
	"path"
	"time"

	"github.com/dgate-io/chi-router"
	"github.com/dgate-io/dgate-api/internal/admin/changestate"
	"github.com/dgate-io/dgate-api/internal/config"
	"github.com/dgate-io/dgate-api/pkg/raftadmin"
	"github.com/dgate-io/dgate-api/pkg/rafthttp"
	"github.com/dgate-io/dgate-api/pkg/storage"
	"github.com/dgate-io/dgate-api/pkg/util"
	"github.com/dgate-io/dgate-api/pkg/util/logadapter"
	"github.com/hashicorp/raft"
	boltdb "github.com/hashicorp/raft-boltdb/v2"
	"go.uber.org/zap"
)

func setupRaft(
	server *chi.Mux,
	logger *zap.Logger,
	conf *config.DGateConfig,
	cs changestate.ChangeState,
) {
	adminConfig := conf.AdminConfig
	var logStore raft.LogStore
	var configStore raft.StableStore
	var snapStore raft.SnapshotStore
	switch conf.Storage.StorageType {
	case config.StorageTypeMemory:
		logStore = raft.NewInmemStore()
		configStore = raft.NewInmemStore()
	case config.StorageTypeFile:
		fileConfig, err := config.StoreConfig[storage.FileStoreConfig](conf.Storage.Config)
		if err != nil {
			panic(fmt.Errorf("invalid config: %s", err))
		}
		raftDir := path.Join(fileConfig.Directory)

		snapStore, err = raft.NewFileSnapshotStore(
			path.Join(raftDir), 5,
			zap.NewStdLog(logger.Named("snap-file")).Writer(),
		)
		if err != nil {
			panic(err)
		}
		if boltStore, err := boltdb.NewBoltStore(
			path.Join(raftDir, "raft.db"),
		); err != nil {
			panic(err)
		} else {
			configStore = boltStore
			logStore = boltStore
		}
	default:
		panic(fmt.Errorf("invalid storage type: %s", conf.Storage.StorageType))
	}

	logger.Info("raft store",
		zap.Stringer("storage_type", conf.Storage.StorageType),
		zap.Any("storage_config", conf.Storage.Config),
	)

	raftConfig := adminConfig.Replication.LoadRaftConfig(
		&raft.Config{
			ProtocolVersion:    raft.ProtocolVersionMax,
			LocalID:            raft.ServerID(adminConfig.Replication.RaftID),
			HeartbeatTimeout:   time.Second * 4,
			ElectionTimeout:    time.Second * 5,
			CommitTimeout:      time.Second * 4,
			BatchApplyCh:       false,
			MaxAppendEntries:   1024,
			LeaderLeaseTimeout: time.Second * 4,

			// TODO: Support snapshots
			SnapshotInterval:  time.Duration(9999 * time.Hour),
			SnapshotThreshold: math.MaxUint64,
			Logger:            logadapter.NewZap2HCLogAdapter(logger),
		},
	)

	advertAddr := adminConfig.Replication.AdvertAddr
	if advertAddr == "" {
		advertAddr = fmt.Sprintf("%s:%d", adminConfig.Host, adminConfig.Port)
	}
	address := raft.ServerAddress(advertAddr)

	raftHttpLogger := logger.Named("http")
	transport := rafthttp.NewHTTPTransport(
		address, http.DefaultClient, raftHttpLogger,
		adminConfig.Replication.AdvertScheme,
	)
	fsmLogger := logger.Named("fsm")
	adminFSM := newAdminFSM(fsmLogger, configStore, cs)
	raftNode, err := raft.NewRaft(
		raftConfig, adminFSM, logStore,
		configStore, snapStore, transport,
	)
	if err != nil {
		panic(err)
	}

	// Setup raft handler
	server.Handle("/raft/*", transport)

	raftAdminLogger := logger.Named("admin")
	raftAdmin := raftadmin.NewServer(
		raftNode, raftAdminLogger,
		[]raft.ServerAddress{address},
	)

	// Setup handler for raft admin
	server.HandleFunc("/raftadmin/*", func(w http.ResponseWriter, r *http.Request) {
		if adminConfig.Replication.SharedKey != "" {
			sharedKey := r.Header.Get("X-DGate-Shared-Key")
			if sharedKey != adminConfig.Replication.SharedKey {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		raftAdmin.ServeHTTP(w, r)
	})

	// Setup handler for stats
	server.Handle("/raftadmin/stats", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Raft-State", raftNode.State().String())
		util.JsonResponse(w, http.StatusOK, raftNode.Stats())
	}))

	// Setup handler for readys
	server.Handle("/raftadmin/readyz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Raft-State", raftNode.State().String())
		if err := cs.WaitForChanges(nil); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		leaderId, leaderAddr := raftNode.LeaderWithID()
		util.JsonResponse(w, http.StatusOK, map[string]any{
			"status":      "ok",
			"proxy_ready": cs.Ready(),
			"state":       raftNode.State().String(),
			"leader":      leaderId,
			"leader_addr": leaderAddr,
		})
	}))

	doer := func(req *http.Request) (*http.Response, error) {
		req.Header.Set("User-Agent", "dgate")
		if adminConfig.Replication.SharedKey != "" {
			req.Header.Set("X-DGate-Shared-Key", adminConfig.Replication.SharedKey)
		}
		client := *http.DefaultClient
		client.Timeout = time.Second * 10
		return client.Do(req)
	}
	adminClient := raftadmin.NewClient(
		doer, logger.Named("raft-admin-client"),
		adminConfig.Replication.AdvertScheme,
	)

	cs.SetupRaft(raftNode, adminClient)

	configFuture := raftNode.GetConfiguration()
	if err = configFuture.Error(); err != nil {
		panic(err)
	}
	serverConfig := configFuture.Configuration()
	raftId := string(raftConfig.LocalID)
	logger.Info("replication config",
		zap.String("raft_id", raftId),
		zap.Any("config", serverConfig),
		zap.Int("max_append_entries", raftConfig.MaxAppendEntries),
		zap.Bool("batch_chan", raftConfig.BatchApplyCh),
		zap.Duration("commit_timeout", raftConfig.CommitTimeout),
		zap.Int("config_proto", int(raftConfig.ProtocolVersion)),
	)

	if adminConfig.Replication.BootstrapCluster && len(serverConfig.Servers) == 0 {
		logger.Info("bootstrapping cluster",
			zap.String("id", raftId),
			zap.String("advert_addr", advertAddr),
		)
		raftNode.BootstrapCluster(raft.Configuration{
			Servers: []raft.Server{
				{
					Suffrage: raft.Voter,
					ID:       raft.ServerID(raftId),
					Address:  raft.ServerAddress(adminConfig.Replication.AdvertAddr),
				},
			},
		})
	} else if len(serverConfig.Servers) == 0 {
		go func() {
			addresses := make([]string, 0)
			if adminConfig.Replication.DiscoveryDomain != "" {
				logger.Debug("no previous configuration found, attempting to discover cluster",
					zap.String("domain", adminConfig.Replication.DiscoveryDomain),
				)
				addrs, err := net.LookupHost(adminConfig.Replication.DiscoveryDomain)
				if err != nil {
					panic(err)
				}
				if len(addrs) == 0 {
					panic(fmt.Errorf("no addrs found for %s", adminConfig.Replication.DiscoveryDomain))
				}
				logger.Info("discovered addresses",
					zap.Strings("addresses", addrs))
				for _, addr := range addrs {
					if addr[len(addr)-1] == '.' {
						addr = addr[:len(addr)-1]
					}
					addresses = append(addresses, fmt.Sprintf("%s:%d", addr, adminConfig.Port))
				}
			}
			logger.Info("no servers found in configuration, adding myself to cluster",
				zap.String("id", raftId),
				zap.String("advert_addr", advertAddr),
				zap.Strings("cluster_addrs", addresses),
			)

			if adminConfig.Replication.ClusterAddrs != nil && len(adminConfig.Replication.ClusterAddrs) > 0 {
				addresses = append(addresses, adminConfig.Replication.ClusterAddrs...)
			}

			if len(addresses) > 0 {
				addresses = append(addresses, adminConfig.Replication.ClusterAddrs...)
				retries := 0
			RETRY:
				for _, addr := range addresses {
					err = adminClient.VerifyLeader(
						context.Background(),
						raft.ServerAddress(addr),
					)
					if err != nil {
						if err == raftadmin.ErrNotLeader {
							continue
						}
						if retries > 15 {
							logger.Error("Skipping verifying leader",
								zap.String("url", addr), zap.Error(err),
							)
							continue
						}
						retries += 1
						logger.Debug("Retrying verifying leader",
							zap.String("url", addr), zap.Error(err))
						<-time.After(3 * time.Second)
						goto RETRY
					}
					// If this node is watch only, add it as a non-voter node, otherwise add it as a voter node
					if adminConfig.WatchOnly {
						logger.Info("Adding non-voter",
							zap.String("id", raftId),
							zap.String("leader", adminConfig.Replication.AdvertAddr),
							zap.String("url", addr),
						)
						resp, err := adminClient.AddNonvoter(
							context.Background(), raft.ServerAddress(addr),
							&raftadmin.AddNonvoterRequest{
								ID:      raftId,
								Address: adminConfig.Replication.AdvertAddr,
							},
						)
						if err != nil {
							panic(err)
						}
						if resp.Error != "" {
							panic(resp.Error)
						}
					} else {
						logger.Info("Adding voter: %s - leader: %s",
							zap.String("id", raftId),
							zap.String("leader", adminConfig.Replication.AdvertAddr),
							zap.String("url", addr),
						)
						resp, err := adminClient.AddVoter(context.Background(), raft.ServerAddress(addr), &raftadmin.AddVoterRequest{
							ID:      raftId,
							Address: adminConfig.Replication.AdvertAddr,
						})
						if err != nil {
							panic(err)
						}
						if resp.Error != "" {
							panic(resp.Error)
						}
					}
					break
				}
				if err != nil {
					panic(err)
				}
			} else {
				logger.Warn("no admin urls specified, waiting to be added to cluster")
			}
		}()
	} else {
		logger.Debug("previous configuration found",
			zap.Any("servers", serverConfig.Servers),
		)
	}
}
