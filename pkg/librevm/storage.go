//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/disk"
	"linuxvm/pkg/filesystem"
	"linuxvm/pkg/static_resources"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

func (v *machineBuilder) applyDiskXattrs(ctx context.Context, rawDiskPath string, xattrs map[string]string) error {
	if len(xattrs) == 0 {
		return nil
	}
	logrus.Infof("updating disk xattrs for %q", rawDiskPath)

	xattr := filesystem.NewXattrManager()
	for key, val := range xattrs {
		if err := xattr.SetXattr(ctx, rawDiskPath, key, val, true); err != nil {
			return fmt.Errorf("setxattr %q=%q on %q: %w", key, val, rawDiskPath, err)
		}
	}

	return nil
}

func (v *machineBuilder) generateRAWDisk(ctx context.Context, rawDiskPath string, givenUUID string, xattrs map[string]string) error {
	rawDiskPath, err := filepath.Abs(filepath.Clean(rawDiskPath))
	if err != nil {
		return err
	}

	diskMgr, err := disk.NewBlkManager()
	if err != nil {
		return err
	}

	if err = static_resources.ExtractEmbeddedRawDisk(ctx, rawDiskPath); err != nil {
		return fmt.Errorf("failed to extract embedded raw disk: %w", err)
	}

	if err = diskMgr.NewUUID(ctx, givenUUID, rawDiskPath); err != nil {
		return fmt.Errorf("failed to write UUID for raw disk %q: %w", rawDiskPath, err)
	}

	return v.applyDiskXattrs(ctx, rawDiskPath, xattrs)
}

func (v *machineBuilder) addRAWDiskToBlkList(ctx context.Context, rawDiskPath string, mountTo string) error {
	rawDiskPath, err := filepath.Abs(filepath.Clean(rawDiskPath))
	if err != nil {
		return err
	}

	diskMgr, err := disk.NewBlkManager()
	if err != nil {
		return err
	}

	info, err := diskMgr.Inspect(ctx, rawDiskPath)
	if err != nil {
		return err
	}

	blkDev := define.BlkDev{
		UUID:    info.UUID,
		FsType:  info.FsType,
		Path:    info.Path,
		MountTo: info.MountTo,
	}
	if mountTo != "" {
		blkDev.MountTo = mountTo
	}

	v.BlkDevs = append(v.BlkDevs, blkDev)

	return nil
}

func (v *machineBuilder) withConfiguredStorageRAWDisk(ctx context.Context, cfg Config) error {
	varDisk := cfg.VarDisk
	varDiskPath, err := filepath.Abs(filepath.Clean(varDisk.RawDiskPath))
	if err != nil {
		return err
	}
	varDisk.RawDiskPath = varDiskPath

	if err = v.reconcileVarRAWDisk(ctx, &varDisk); err != nil {
		return err
	}
	if err = v.addRAWDiskToBlkList(ctx, varDisk.RawDiskPath, varDisk.Mnt); err != nil {
		return err
	}

	for _, diskSpec := range cfg.ExternalDisks {
		if diskSpec.RawDiskPath == "" {
			return fmt.Errorf("raw disk path is empty")
		}
		rawDiskPath, err := filepath.Abs(filepath.Clean(diskSpec.RawDiskPath))
		if err != nil {
			return err
		}
		diskSpec.RawDiskPath = rawDiskPath

		if _, err = os.Stat(rawDiskPath); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("stat external raw disk %q failed: %w", rawDiskPath, err)
			}

			logrus.Warnf("external raw disk %q not found, creating...", rawDiskPath)

			if diskSpec.UUID == "" {
				diskSpec.UUID = uuid.NewString()
			}

			if err := v.generateRAWDisk(ctx, diskSpec.RawDiskPath, diskSpec.UUID, map[string]string{}); err != nil {
				return fmt.Errorf("failed to create external raw disk %q: %w", rawDiskPath, err)
			}
		}

		logrus.Infof("attaching external raw disk %q", rawDiskPath)
		if err = v.addRAWDiskToBlkList(ctx, diskSpec.RawDiskPath, diskSpec.Mnt); err != nil {
			return err
		}
	}

	return nil
}

func (v *machineBuilder) reconcileVarRAWDisk(ctx context.Context, diskSpec *RawDisk) error {
	var versionXattrs map[string]string
	if diskSpec.Version != "" {
		versionXattrs = map[string]string{
			define.XattrDiskVersionKey: diskSpec.Version,
		}
	}

	rawDiskPath := diskSpec.RawDiskPath
	if _, err := os.Stat(rawDiskPath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		logrus.Infof("var disk %q not found, creating new disk", rawDiskPath)
		diskSpec.UUID = define.VarDataDiskUUID
		return v.generateRAWDisk(ctx, rawDiskPath, define.VarDataDiskUUID, versionXattrs)
	}

	shouldRegenerate, hasVersionXattr, err := v.needsDiskRegeneration(ctx, rawDiskPath, diskSpec.Version)
	if err != nil {
		return err
	}
	if !hasVersionXattr || shouldRegenerate {
		if shouldRegenerate {
			logrus.Warnf("var disk %q needs regeneration", rawDiskPath)
		} else {
			logrus.Warnf("var disk %q has no version xattr, regenerating", rawDiskPath)
		}
		diskSpec.UUID = define.VarDataDiskUUID
		return v.recreateRAWDisk(ctx, rawDiskPath, define.VarDataDiskUUID, versionXattrs)
	}

	logrus.Infof("var disk %q version is up-to-date, skip xattr update", rawDiskPath)
	return nil
}

func (v *machineBuilder) recreateRAWDisk(ctx context.Context, rawDiskPath string, uuid string, xattrs map[string]string) error {
	logrus.Warnf("removing existing disk: %q", rawDiskPath)
	if err := os.Remove(rawDiskPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return v.generateRAWDisk(ctx, rawDiskPath, uuid, xattrs)
}

func (v *machineBuilder) needsDiskRegeneration(ctx context.Context, diskPath string, expected string) (bool, bool, error) {
	if expected == "" {
		return false, false, nil
	}

	xattr := filesystem.NewXattrManager()
	stored, err := xattr.GetXattr(ctx, diskPath, define.XattrDiskVersionKey)
	if err != nil {
		logrus.Debugf("read disk version xattr for %q: %v", diskPath, err)
	}

	if stored == "" {
		logrus.Infof("disk %q has no version xattr", diskPath)
		return false, false, nil
	}

	logrus.Infof("stored disk %q version: %q, expected version: %q", diskPath, stored, expected)
	return stored != expected, true, nil
}
