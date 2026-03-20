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
)

func (v *machineBuilder) generateRAWDisk(ctx context.Context, rawDiskPath string, givenUUID string) error {
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

	xattr := filesystem.NewXattrManager()

	for key, val := range v.DiskXattrs {
		if err = xattr.SetXattr(ctx, rawDiskPath, key, val, true); err != nil {
			return fmt.Errorf("setxattr %q=%q on %q: %w", key, val, rawDiskPath, err)
		}
	}

	return nil
}

func (v *machineBuilder) configureContainerRAWDisk(ctx context.Context, diskPath string, version string) error {
	v.withRAWDiskVersionXATTR(version)

	if v.needsDiskRegeneration(ctx, diskPath) {
		if err := os.Remove(diskPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	if _, err := os.Stat(diskPath); err != nil {
		if err = v.generateRAWDisk(ctx, diskPath, define.ContainerDiskUUID); err != nil {
			return fmt.Errorf("failed to generate container storage raw disk: %w", err)
		}
	}

	return v.addRAWDiskToBlkList(ctx, diskPath)
}

func (v *machineBuilder) addRAWDiskToBlkList(ctx context.Context, rawDiskPath string) error {
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

	v.BlkDevs = append(v.BlkDevs, blkDev)

	return nil
}

func (v *machineBuilder) withUserProvidedStorageRAWDisk(ctx context.Context, disks map[string]string) error {
	for diskFile, diskUUID := range disks {
		if diskFile == "" {
			return fmt.Errorf("raw disk path is empty")
		}

		rawDiskPath, err := filepath.Abs(filepath.Clean(diskFile))
		if err != nil {
			return err
		}
		if _, err = os.Stat(rawDiskPath); err != nil {
			if diskUUID == "" {
				diskUUID = uuid.NewString()
			}
			if err = v.generateRAWDisk(ctx, rawDiskPath, diskUUID); err != nil {
				return err
			}
		}

		if err = v.addRAWDiskToBlkList(ctx, rawDiskPath); err != nil {
			return err
		}
	}

	return nil
}

func (v *machineBuilder) needsDiskRegeneration(ctx context.Context, diskPath string) bool {
	xattrKey := define.XattrDiskVersionKey
	xattr := filesystem.NewXattrManager()

	stored, _ := xattr.GetXattr(ctx, diskPath, xattrKey)
	expected := v.DiskXattrs[xattrKey]

	return stored != expected
}

func (v *machineBuilder) withRAWDiskVersionXATTR(value string) *machineBuilder {
	v.DiskXattrs = map[string]string{
		define.XattrDiskVersionKey: value,
	}
	return v
}
