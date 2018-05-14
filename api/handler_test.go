package api

import (
	"testing"
	"time"

	"github.com/docker/swarm/garden/database"
)

func TestGetNFSBackupFile(t *testing.T) {
	mounts := `rootfs / rootfs rw 0 0
udev /dev tmpfs rw,relatime,size=16440620k,nr_inodes=0,mode=755 0 0
tmpfs /dev/shm tmpfs rw,relatime,size=16440620k,nr_inodes=4110155 0 0
/dev/hda1 / ext3 rw,relatime,errors=continue,user_xattr,acl,barrier=1,data=ordered 0 0
proc /proc proc rw,relatime 0 0
sysfs /sys sysfs rw,relatime 0 0
devpts /dev/pts devpts rw,relatime,gid=5,mode=620,ptmxmode=000 0 0
debugfs /sys/kernel/debug debugfs rw,relatime 0 0
/dev/hda3 /boot ext3 rw,relatime,errors=continue,user_xattr,acl,barrier=1,data=ordered 0 0
/dev/hda5 /LSYSFIL ext3 rw,relatime,errors=continue,barrier=1,data=ordered 0 0
fusectl /sys/fs/fuse/connections fusectl rw,relatime 0 0
securityfs /sys/kernel/security securityfs rw,relatime 0 0
none /proc/sys/fs/binfmt_misc binfmt_misc rw,relatime 0 0
146.32.99.17:/SHARED /SHARED nfs rw,relatime,vers=3,rsize=1048576,wsize=1048576,namlen=255,hard,nolock,proto=tcp,timeo=600,retrans=2,sec=sys,mountaddr=146.32.99.17,mountvers=3,mountport=20048,mountproto=udp,local_lock=all,addr=146.32.99.17 0 0
146.240.104.26:/DBAASNFS /DBAASNFS nfs rw,relatime,vers=3,rsize=1048576,wsize=1048576,namlen=255,hard,nolock,proto=tcp,timeo=600,retrans=2,sec=sys,mountaddr=146.240.104.26,mountvers=3,mountport=20048,mountproto=udp,local_lock=all,addr=146.240.104.26 0 0
146.240.104.27:/DBAASNFS /DBAASNFS nfs rw,relatime,vers=3,rsize=1048576,wsize=1048576,namlen=255,hard,nolock,proto=tcp,timeo=600,retrans=2,sec=sys,mountaddr=146.240.104.26,mountvers=3,mountport=20048,mountproto=udp,local_lock=all,addr=146.240.104.26 0 0
xxxxxxx
`

	file := "/BACKUP/3675b7f8_fdsre001/dbbackup/3675b7f8_fdsre001_201802101607"
	want := "/DBAASNFS/3675b7f8_fdsre001/dbbackup/3675b7f8_fdsre001_201802101607"

	got := getNFSBackupFile(file, "/BACKUP", "146.240.104.26:/DBAASNFS", mounts)
	if got != want {
		t.Errorf("expected %s but got %s", want, got)
	}
}

func TestExpiredFiles(t *testing.T) {
	now := time.Now()
	files := []database.BackupFile{
		{ID: "0", Retention: now.Add(-48 * time.Hour)},
		{ID: "1", Retention: now.Add(-24 * time.Hour)},
		{ID: "2", Retention: now.Add(-time.Hour)},
		{ID: "3", Retention: now.Add(time.Hour)},
		{ID: "4", Retention: now.Add(24 * time.Hour)},
	}

	ex0 := expiredFiles(files, "")
	if got := len(ex0); got != 3 {
		t.Errorf("%s,got %d files but want %d", now, got, 3)
	}

	date := now.Add(-25 * time.Hour).Format(dateLayout)
	ex1 := expiredFiles(files, date)
	if got := len(ex1); got != 1 {
		t.Errorf("%s,got %d files but want %d", date, got, 1)
	}

	date = now.Add(24 * time.Hour).Format(dateLayout)
	ex2 := expiredFiles(files, date)
	if got := len(ex2); got != 3 {
		t.Errorf("%s,got %d files but want %d", date, got, 3)
	}
}
