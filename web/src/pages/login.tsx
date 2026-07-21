import { FormEvent, useEffect, useState } from "react";
import { Activity } from "lucide-react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Spinner } from "@/components/ui/spinner";
import { login } from "@/lib/auth";
import {
  clearRememberedCredentials,
  getCredentialStorage,
  getRememberedCredentials,
  saveRememberedCredentials,
} from "@/lib/remembered-credentials";

/** 渲染后台登录页面并管理浏览器记住密码状态。 */
export function LoginPage() {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [rememberPassword, setRememberPassword] = useState(false);
  const [error, setError] = useState("");
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    const credentials = getRememberedCredentials(getCredentialStorage(window));
    if (!credentials) {
      return;
    }

    setUsername(credentials.username);
    setPassword(credentials.password);
    setRememberPassword(true);
  }, []);

  /** 处理记住密码状态，并在取消时立即清除旧记录。 */
  function handleRememberPasswordChange(checked: boolean) {
    setRememberPassword(checked);
    if (!checked) {
      clearRememberedCredentials(getCredentialStorage(window));
    }
  }

  /** 验证账号密码，登录成功后按勾选状态更新浏览器记录。 */
  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setIsLoading(true);
    try {
      await login(username, password);
      if (rememberPassword) {
        saveRememberedCredentials(getCredentialStorage(window), { username, password });
      } else {
        clearRememberedCredentials(getCredentialStorage(window));
      }
      window.location.href = "/";
    } catch (err) {
      setError(err instanceof Error ? err.message : "登录失败");
    } finally {
      setIsLoading(false);
    }
  }

  return (
    <div className="min-h-screen bg-background">
      <div className="grid min-h-screen lg:grid-cols-[1fr_520px]">
        <section className="hidden border-r bg-muted/30 px-10 py-12 lg:flex lg:flex-col lg:justify-between">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-md bg-foreground text-background">
              <Activity className="h-5 w-5" />
            </div>
            <div>
              <div className="font-semibold">Aowugong 工作台</div>
              <div className="text-sm text-muted-foreground">Personal finance automation workspace</div>
            </div>
          </div>
          <div className="max-w-xl">
            <div className="mb-4 text-5xl font-semibold tracking-normal">安静、清楚、可掌控。</div>
            <p className="text-base leading-7 text-muted-foreground">
              数据同步、策略回测、定时任务和交易提醒统一放在一个后台里。界面只服务于判断和行动。
            </p>
          </div>
          <div className="grid grid-cols-3 gap-3 text-sm">
            <div className="rounded-md border bg-background p-4">
              <div className="tabular-nums text-xl font-semibold">20:00</div>
              <div className="mt-1 text-muted-foreground">日线补数</div>
            </div>
            <div className="rounded-md border bg-background p-4">
              <div className="tabular-nums text-xl font-semibold">2345</div>
              <div className="mt-1 text-muted-foreground">统一端口</div>
            </div>
            <div className="rounded-md border bg-background p-4">
              <div className="tabular-nums text-xl font-semibold">Go</div>
              <div className="mt-1 text-muted-foreground">统一服务</div>
            </div>
          </div>
        </section>
        <section className="flex items-center justify-center px-4 py-10">
          <Card className="w-full max-w-sm">
            <CardHeader>
              <CardTitle>登录</CardTitle>
              <CardDescription>使用后台账号进入个人工作台。</CardDescription>
            </CardHeader>
            <CardContent>
              <form className="space-y-4" onSubmit={handleSubmit}>
                <div className="space-y-2">
                  <Label htmlFor="username">用户名</Label>
                  <Input
                    id="username"
                    autoComplete="username"
                    value={username}
                    onChange={(event) => setUsername(event.target.value)}
                    required
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="password">密码</Label>
                  <Input
                    id="password"
                    type="password"
                    autoComplete="current-password"
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                    required
                  />
                </div>
                <div className="flex items-center gap-2">
                  <Checkbox
                    id="remember-password"
                    checked={rememberPassword}
                    onCheckedChange={(checked) => handleRememberPasswordChange(checked === true)}
                  />
                  <Label className="cursor-pointer font-normal" htmlFor="remember-password">
                    记住密码
                  </Label>
                </div>
                {error ? (
                  <Alert variant="destructive">
                    <AlertDescription>{error}</AlertDescription>
                  </Alert>
                ) : null}
                <Button className="w-full" type="submit" disabled={isLoading}>
                  {isLoading ? <Spinner /> : null}
                  登录
                </Button>
              </form>
            </CardContent>
          </Card>
        </section>
      </div>
    </div>
  );
}
