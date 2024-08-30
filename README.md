# ITM影像批量下载程序

```bash
Uasage of itm-downloader:
  -f string
        facility参数的短写
  -facility string
        登陆所属facility 
         对应环境变量FACILITY
  -o string
        outDir参数的短写 (default "./")
  -outDir string
        下载目录 
         对应环境变量OUTDIR
  -p string
        password参数的短写
  -password string
        登陆密码 
         对应环境变量PASSWORD
  -u string
        username参数的短写
  -username string
        登陆用户名 
         对应环境变量USERNAME
```

例子：

```bash
# 下载到 当前目录
itm-downloader -f testing -u testing@lab.local -p "$RFV1qaz"
```

```bash
# 使用环境变量
export FACILITY=testing
export USERNAME=testing@lab.local
export PASSWORD="$RFV1qaz""
itm-downloader
```

```bash
# 使用.env文件保存环境变量
cat > .env <<EOF
FACILITY=testing
USERNAME=testing@lab.local
PASSWORD="$RFV1qaz"
EOF

itm-downloader
```

* 上述参数用法可以混合使用，比如

  ```bash
  # 同时使用了环境变量、.env、参数（长短参数名）
  export PASSWORD="$RFV1qaz"
  cat > .env <<EOF
  USERNAME=testing@lab.local
  FACILITY=testing
  EOF
  itm-downloader -o $HOME/testing
  ```
* 参数没有顺序
* 优先级：参数>环境变量>.env文件
* 任何不填写的参数，会检查：

  * 是否有环境变量，有就使用
  * 没有对应环境变量就检查.env文件，有就使用
  * 都没有就检查该参数可缺省，有就使用默认值，没有就报错
