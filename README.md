# test-nginx-with-go-and-docker

これは、nginx を Go と Docker で自動テストするサンプルです。

- `sample-nginx/`
    - テスト対象となる nginx の設定ファイルや Dockerfile です。
- `test/`
    - nginx をテストするための Go プログラムやテストの起動スクリプトです。

## テストの実行方法

テストを実行するには、リポジトリのトップで `make test` を実行してください。
テストの実行には docker が必要です。

## テストコードの説明

- `nginx_test.go`
    - テストケースが記述されているコードです。
- `nginx.go`
    - テスト対象となる nginx を起動したり終了したりするコードです。内部で docker コマンドを叩いています。
- `mock_ap.go`
    - APサーバーのモックを起動したり終了したりするコードです。
- `run`
    - テスト実行を開始するためのシェルスクリプトです。

## テストを実行する環境

ローカル環境でも CircleCI 環境でも実行できるようにするために、テストは以下のような構成になっています。

![architecture](./architecture.png)

太い青枠で囲われた部分が Docker コンテナです。

テストは nginx-tester というコンテナの中で実行されます。
それぞれのテストはテスト対象となる nginx コンテナを起動します。

nginx-tester と nginx が相互に通信できるようにするために、nginx コンテナは nginx-tester と同じ docker network 内に配置します。
この docker network は `run` というシェルスクリプトがテストの起動前に作成します。

AP のモックサーバーは独立したコンテナではなく、nginx-tester 内の goroutine として起動します。

テストケースは並列に実行されるので、nginx のコンテナや MockAP は複数個同時に実行されます。nginx のコンテナに suffix が付けられているのはそのためです。
