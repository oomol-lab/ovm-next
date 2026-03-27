# How to use rsync to sync code to/from the raspberry pi

```shell
# please make sure you have the following environment variables set
export SSH_PORT=10024
export SSH_REMOTE_ADDR="ihexon@192.168.1.210:/home/ihexon/revm/"   # please include / at the end
export LOCAL_SRC_DIR="/mnt/c/Users/localuser/GolandProjects/revm/" # please include / at the end

# pull code from raspberry pi
rsync -avz --delete  --no-perms --exclude='revm-*' --exclude=.git --exclude='.idea/' --exclude='out/' -e "ssh -p $SSH_PORT" "$SSH_REMOTE_ADDR" "$LOCAL_SRC_DIR"

# push code to raspberry pi
rsync -avz --delete  --no-perms --exclude='revm-*' --exclude=.git --exclude='.idea/' --exclude='out/' -e "ssh -p $SSH_PORT" "$LOCAL_SRC_DIR" "$SSH_REMOTE_ADDR"
```
