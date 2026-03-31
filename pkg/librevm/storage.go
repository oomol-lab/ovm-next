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

func absDiskPath(rawDiskPath string) (string, error) {
	if rawDiskPath == "" {
		return "", fmt.Errorf("raw disk path is empty")
	}
	return filepath.Abs(filepath.Clean(rawDiskPath))
}

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

func (v *machineBuilder) withVarDisk(ctx context.Context, diskSpec RawDisk) error {
	rawDiskPath, err := absDiskPath(diskSpec.RawDiskPath)
	if err != nil {
		return err
	}
	diskSpec.RawDiskPath = rawDiskPath
	logrus.Infof("preparing var disk path=%q version=%q uuid=%q mount=%q",
		diskSpec.RawDiskPath, diskSpec.Version, define.VarDataDiskUUID, define.VarDiskMountPoint)

	if err = v.reconcileVarRAWDisk(ctx, &diskSpec); err != nil {
		return err
	}

	logrus.Infof("attaching var disk path=%q mount=%q", diskSpec.RawDiskPath, diskSpec.Mnt)
	return v.addRAWDiskToBlkList(ctx, diskSpec.RawDiskPath, diskSpec.Mnt)
}

func (v *machineBuilder) withUserProvidedRawDisk(ctx context.Context, diskSpecs []RawDisk) error {
	if len(diskSpecs) == 0 {
		logrus.Info("no user-provided raw disks configured")
		return nil
	}
	logrus.Infof("preparing %d user-provided raw disk(s)", len(diskSpecs))

	for _, diskSpec := range diskSpecs {
		rawDiskPath, err := absDiskPath(diskSpec.RawDiskPath)
		if err != nil {
			return err
		}
		diskSpec.RawDiskPath = rawDiskPath
		logrus.Infof("processing external raw disk path=%q version=%q uuid=%q mount=%q",
			diskSpec.RawDiskPath, diskSpec.Version, diskSpec.UUID, diskSpec.Mnt)

		if err := v.reconcileUserProvidedRawDisk(ctx, &diskSpec); err != nil {
			return err
		}

		logrus.Infof("attaching external raw disk path=%q uuid=%q mount=%q",
			diskSpec.RawDiskPath, diskSpec.UUID, diskSpec.Mnt)
		if err = v.addRAWDiskToBlkList(ctx, diskSpec.RawDiskPath, diskSpec.Mnt); err != nil {
			return err
		}
	}

	return nil
}

func (v *machineBuilder) reconcileUserProvidedRawDisk(ctx context.Context, diskSpec *RawDisk) error {
	_, statErr := os.Stat(diskSpec.RawDiskPath)
	if statErr == nil {
		logrus.Infof("external raw disk exists path=%q", diskSpec.RawDiskPath)
		return v.handleExistingUserProvidedRawDisk(ctx, diskSpec)
	}
	if !os.IsNotExist(statErr) {
		return fmt.Errorf("stat external raw disk %q failed: %w", diskSpec.RawDiskPath, statErr)
	}

	logrus.Infof("external raw disk not found path=%q", diskSpec.RawDiskPath)
	return v.createUserProvidedRawDisk(ctx, diskSpec)
}

func (v *machineBuilder) handleExistingUserProvidedRawDisk(ctx context.Context, diskSpec *RawDisk) error {
	diskMgr, err := disk.NewBlkManager()
	if err != nil {
		return err
	}

	info, err := diskMgr.Inspect(ctx, diskSpec.RawDiskPath)
	if err != nil {
		return fmt.Errorf("inspect external raw disk %q failed: %w", diskSpec.RawDiskPath, err)
	}

	// Existing disk must keep its own UUID; ignore CLI-provided UUID.
	diskSpec.UUID = info.UUID
	if diskSpec.Mnt == "" {
		diskSpec.Mnt = info.MountTo
	}

	shouldRegenerate, hasVersionXattr, err := v.needsDiskRegeneration(ctx, diskSpec.RawDiskPath, diskSpec.Version)
	if err != nil {
		return err
	}
	if !hasVersionXattr {
		logrus.Infof("external raw disk %q has no version xattr, skip version bump", diskSpec.RawDiskPath)
		return nil
	}
	if !shouldRegenerate {
		logrus.Infof("external raw disk %q version is up-to-date", diskSpec.RawDiskPath)
		return nil
	}

	logrus.Warnf("external raw disk %q needs regeneration", diskSpec.RawDiskPath)
	return v.recreateRAWDisk(ctx, diskSpec.RawDiskPath, diskSpec.UUID, diskVersionXattrs(diskSpec.Version))
}

func (v *machineBuilder) createUserProvidedRawDisk(ctx context.Context, diskSpec *RawDisk) error {
	logrus.Infof("creating external raw disk path=%q", diskSpec.RawDiskPath)
	if diskSpec.UUID == "" {
		diskSpec.UUID = uuid.NewString()
	}
	if diskSpec.Mnt == "" {
		diskSpec.Mnt = fmt.Sprintf("/mnt/%s", diskSpec.UUID)
	}
	logrus.Infof("new external raw disk metadata path=%q uuid=%q mount=%q version=%q",
		diskSpec.RawDiskPath, diskSpec.UUID, diskSpec.Mnt, diskSpec.Version)

	if err := v.generateRAWDisk(ctx, diskSpec.RawDiskPath, diskSpec.UUID, diskVersionXattrs(diskSpec.Version)); err != nil {
		return fmt.Errorf("failed to create external raw disk %q: %w", diskSpec.RawDiskPath, err)
	}
	return nil
}

func (v *machineBuilder) reconcileVarRAWDisk(ctx context.Context, diskSpec *RawDisk) error {
	diskSpec.UUID = define.VarDataDiskUUID

	versionXattrs := diskVersionXattrs(diskSpec.Version)
	logrus.Infof("reconciling var disk path=%q version=%q", diskSpec.RawDiskPath, diskSpec.Version)

	rawDiskPath := diskSpec.RawDiskPath
	if _, err := os.Stat(rawDiskPath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		logrus.Infof("var disk not found path=%q creating with version=%q", rawDiskPath, diskSpec.Version)
		return v.generateRAWDisk(ctx, rawDiskPath, define.VarDataDiskUUID, versionXattrs)
	}

	shouldRegenerate, hasVersionXattr, err := v.needsDiskRegeneration(ctx, rawDiskPath, diskSpec.Version)
	if err != nil {
		return err
	}
	if !hasVersionXattr || shouldRegenerate {
		if shouldRegenerate {
			logrus.Infof("var disk version mismatch path=%q expected=%q regenerating", rawDiskPath, diskSpec.Version)
		} else {
			logrus.Infof("var disk has no version xattr path=%q regenerating", rawDiskPath)
		}
		return v.recreateRAWDisk(ctx, rawDiskPath, define.VarDataDiskUUID, versionXattrs)
	}

	logrus.Infof("var disk is up-to-date path=%q version=%q", rawDiskPath, diskSpec.Version)
	return nil
}

func (v *machineBuilder) recreateRAWDisk(ctx context.Context, rawDiskPath string, uuid string, xattrs map[string]string) error {
	logrus.Warnf("removing existing disk: %q", rawDiskPath)
	if err := os.Remove(rawDiskPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return v.generateRAWDisk(ctx, rawDiskPath, uuid, xattrs)
}

func diskVersionXattrs(version string) map[string]string {
	if version == "" {
		return nil
	}
	return map[string]string{
		define.XattrDiskVersionKey: version,
	}
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
