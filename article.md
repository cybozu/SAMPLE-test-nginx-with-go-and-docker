# 複雑怪奇な nginx を Go と Docker で自動テストする

全国の nginx 職人のみなさま、こんにちは。野島([@nojima](https://twitter.com/nojima))です。

nginx のテストを Go と Docker で行ってみたところ、かなり良かったので紹介したいと思います。

## 背景

我々のプロダクションにおける nginx の設定は、あらゆる要件を吸い取った結果、複雑怪奇なものになっています。
nginx の設定を書く行為はいつしか "nginx芸" と呼ばれるようになりました。
一部の人間しかできないからです。
そもそも、何が壊れるかわからないようなものを誰も変更したがらないのです。
誰でも安心して nginx の設定を変更できるような仕組みが必要でした。

一応、昔から nginx の自動テストはありました。
しかしこのテストを動かすためには nginx をデータセンターにデプロイせねばならず、さらに複雑なセットアップ手順を **手動** で行う必要がありました。
結局このテストはほとんど使われませんでした。

現在では nginx を docker の上に構築するようになったので、新しいテストが可能となりました。
このテストはデータセンターにデプロイすることなくローカルで実行でき、特別なセットアップも不要です。
また、モックを駆使することで異常系も簡単にテストすることができます。

この記事ではこの新しいテストについて説明していきたいと思います。

## サンプルコード

この記事のために説明用のサンプルコードを用意しました。
ローカルに clone してきて `make test` するとテストを実行できます。

TODO: URL を貼る。

## テスト対象の nginx

今回テスト対象とする nginx は以下のような設定になっています（一部抜粋）。
APサーバーにリバプロするエンドポイントや、常に 400 を返すエンドポイントなどがあります。

```nginx
server {
	listen 80;

	location / {
		proxy_pass http://${AP_SERVER_ADDR};
		...
	}

	location /secret/ {
		deny all;
	}

	...
}
```

注目してほしいのは、`proxy_pass` の部分です。
APサーバーのアドレスを直接設定ファイルに埋め込むのではなく、`AP_SERVER_ADDR` という環境変数に切り出しています。
これは、テストする際にAPサーバーをモックサーバーに置き換える必要があるためです。

切り出した環境変数は、コンテナ起動時に [envsubst][envsubst] で具体的な値に展開します。
サンプルコードでは entrypoint がその作業をやっています。 // TODO: リンク

[envsubst]: https://www.gnu.org/software/gettext/manual/gettext.html#envsubst-Invocation

## テストの概観

テストコードの説明に入る前に、テストの概観を図を使って説明します。
ローカル環境でも CircleCI 環境でも実行できるようにするために、テストは以下のような構成になっています。

![architecture](./architecture.png)

太い青枠で囲われた部分が Docker コンテナを表しています。nginx-tester と nginx という２種類のコンテナがあります。nginx のコンテナに `-xxxxxx` という suffix が付いているのは、ランダムな suffix が付与されることを表しています。

nginx-tester は `go test -v ./...` を実行するコンテナです。
テストの実行には Go と docker が必要なので、nginx-tester のイメージには `circleci/golang:1.14` を使っています。

nginx コンテナは、テスト対象となる nginx を格納しているコンテナです。
サンプルコードでは、`sample-nginx:latest` というイメージです。
nginx コンテナは、テストプログラムが動的に起動・終了させます。
テストケースごとに専用の nginx コンテナを立てるので、nginx コンテナは複数個起動することになります。

nginx-tester と nginx が相互に通信できるようにするために、nginx コンテナと nginx-tester は同じ docker network に所属しています。
この docker network はテストの起動前にシェルスクリプトで作成しておきます。

AP のモックサーバーは独立したコンテナではなく、nginx-tester 内の goroutine として起動します。図の破線で囲われた部分が goroutine を表しています。

テストケースは並列に実行されるので、nginx のコンテナや MockAP は複数個同時に実行されます。nginx のコンテナ名に suffix が付けられているのは、同時実行したときに名前が被るのを防ぐためです。

さて、テストの概観がわかったところで実際のテストコードを見ていきましょう。
まずはより簡単なリバプロなしの場合から始めます。

## テストコード (リバプロなし)

次のテストは `GET /secret/` で 400 が返ってくることを確認するものです。

```go
func TestSecretEndpoints(t *testing.T) {
	t.Parallel()

	nginx := StartNginx(t, NginxConfig{}) // ①
	defer nginx.Close(t)
	nginx.Wait(t)
	
	resp, err := http.Get(nginx.URL() + "/secret/") // ②
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden { // ③
		t.Errorf("status code should be 400, but %d", resp.StatusCode)
	}
}
```

① `StartNginx()` は nginx コンテナを起動する関数です。詳細は後述します。 
次に `nginx.Wait()` で起動が完了するまで待ちます。

② nginx に対して `GET /secret/` を行います。`http.Get()` は単なる Go の標準関数です。

③ レスポンスを assert します。これも普通の Go のテストコードです。

前節では Docker を使ってテストすると説明しましたが、Docker はこのテストケースのコードからは完全に隠蔽されています。
これは、Docker にまつわる複雑さをテストケースから分離することで、テストケースを読みやすくするためです。

テストコードでは関心の分離や SRP が蔑ろにされることがよくありますが、私はテストコードでもこれらの原則は重要であると考えています。

Docker を実際に操作しているのは `StartNginx()` や `nginx.Close()` などの関数です。
それでは `StartNginx()` の実装を見ていきましょう。

## StartNginx()

`StartNginx()` の肝となる部分は以下のコードです。この関数の主な仕事は docker コマンドを叩くことです。

```go
// docker コマンドを叩いて sample-nginx:latest を起動する。
args := []string{
	"run", "--rm",
	"--name", name,
	"--net", network,
	"-e", fmt.Sprintf("AP_SERVER_ADDR=%s", config.APServerAddress),
	"sample-nginx:latest",
}
cmd := exec.Command("docker", args...)
if err := cmd.Start(); err != nil {
	t.Fatal(err)
}
```

このコードで注目してほしい点は、`-e` で `AP_SERVER_ADDR` に値を渡していることです。
これにより、任意のAPサーバーを差し込んで nginx を起動できるわけです。
それでは、実際にAPサーバーを差し込むテストを見ていきましょう。

## テストコード (リバプロあり)

次のテストは AP サーバーをモックしてリバプロの挙動を確認するものです。

```go
func TestReverseProxy(t *testing.T) {
	t.Parallel()

	ap := StartMockAP(t) // ①
	defer ap.Close(t)

	nginx := StartNginx(t, NginxConfig{ // ②
		APServerAddress: ap.Address(),
	})
	defer nginx.Close(t)
	nginx.Wait(t)

	resp, err := http.Get(nginx.URL() + "/") // ③
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status code should be 200, but %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "I am AP server" {
		t.Errorf("unexpected response body: %s", string(body))
	}
}
```

① `StartMockAP()` でモックAPを起動しています。

② `StartNginx()` で nginx コンテナを起動します。ここでモックAPのアドレスを差し込んでいることに注目してください。

③ AP と nginx を起動できたら、あとはもう普通のテストです。リクエストを送り、レスポンスを普通に assert しましょう。

MockAP を Nginx に差し込んでいる部分は OOP における Dependency Injection に相当します。
普通、Dependecy Injection は同じプロセス内のオブジェクトに対して行うのですが、この例だと別のプロセスに対して依存オブジェクトを差し込んでいる形になっているのが面白いところです。

それでは最後に `StartMockAP()` の実装を見てみましょう。

## StartMockAP()

`StartMockAP()` は以下のようになっています（一部説明に不要な部分を省略しています）。
ポートを自動的に選ぶためにちょっと特殊なことをしていることを除けば、単に goroutine で HTTP サーバーを立てているだけです。 

```go
// 空いているポートを自動的に選ぶ
l, err := net.Listen("tcp", ":0")
if err != nil {
	t.Fatal(err)
}

handler := func(w http.ResponseWriter, req *http.Request) {
	w.Write([]byte("I am AP server"))
}
ap := &MockAP{
	host: host,
	port: l.Addr().(*net.TCPAddr).Port,
	server: &http.Server{
		Handler: http.HandlerFunc(handler),
	},
}

// 別の goroutine でサーバーを走らせる
go func() {
	if err := ap.server.Serve(l); err != nil && err != http.ErrServerClosed {
		t.Log(err)
	}
}()
```

なお、標準ライブラリの `httptest` で `StartMockAP` と同じようなことができますが、`httptest` はアドレスを `127.0.0.1` にバインドしてしまうので、今回のユースケースでは利用できません。

今回のサンプルコードでは一種類の MockAP しか実装されていませんが、私達が実際に使っているテストでは様々な MockAP が実装されています。
例えば、レスポンスを一切返さない MockAP や、不正な SSL 証明書を持つ MockAP などがあります。
これらの MockAP を使うことで、手動で起こすのが面倒なケースをテストすることができます。

## まとめ

Go と Docker を使って nginx をテストする仕組みを紹介しました。
この仕組みは、我々のプロダクション環境における nginx を支えています。

この記事がみなさまの nginx ライフの一助となれば幸いです。
