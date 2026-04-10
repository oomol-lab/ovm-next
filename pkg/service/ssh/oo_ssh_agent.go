package ssh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	ooSSHAgentIdentity "github.com/oomol-lab/ovm-ssh-agent/v3/pkg/identity"
	ooSSHAgent "github.com/oomol-lab/ovm-ssh-agent/v3/pkg/sshagent"
	ooSSHAgentUtil "github.com/oomol-lab/ovm-ssh-agent/v3/pkg/system"
	"github.com/sirupsen/logrus"
)

const OOSSHAgentGuestSocket = "/opt/ssh_auth/oo-ssh-agent.sock"

type OOSSHAgentService struct {
	upstreamSocket string
	localSocket    string
}

func NewOOSSHAgentService(localSocketPath string) *OOSSHAgentService {
	return &OOSSHAgentService{
		upstreamSocket: ooSSHAgentUtil.GetSSHAgent(),
		localSocket:    localSocketPath,
	}
}

func (s *OOSSHAgentService) Enabled() bool {
	return s != nil && s.upstreamSocket != ""
}

func (s *OOSSHAgentService) LocalSocketPath() string {
	if s == nil {
		return ""
	}
	return s.localSocket
}

func (s *OOSSHAgentService) GuestSocketPath() string {
	return OOSSHAgentGuestSocket
}

func (s *OOSSHAgentService) ForwardSpec() string {
	return fmt.Sprintf("%s:%s", s.GuestSocketPath(), s.LocalSocketPath())
}

func (s *OOSSHAgentService) Run(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if !s.Enabled() {
		logrus.Info("no upstream SSH agent found, skip builtin SSH agent forwarding")
		return nil
	}
	if s.localSocket == "" {
		return errors.New("local SSH agent socket path is empty")
	}

	if err := os.MkdirAll(filepath.Dir(s.localSocket), 0755); err != nil {
		return fmt.Errorf("create ssh agent socket dir: %w", err)
	}
	if err := os.Remove(s.localSocket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale ssh agent socket %q: %w", s.localSocket, err)
	}
	defer func() { _ = os.Remove(s.localSocket) }()

	sshAgent := ooSSHAgent.NewSSHAgent(ctx, s.upstreamSocket, s.localSocket)
	keys := ooSSHAgentIdentity.FindPrivateKeys()
	sshAgent.LoadLocalKeys(keys...)

	logrus.Infof("starting oo ssh agent proxy in %q (upstream: %q)", s.localSocket, s.upstreamSocket)
	if err := sshAgent.Serve(); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil
		}
		return err
	}
	return nil
}
