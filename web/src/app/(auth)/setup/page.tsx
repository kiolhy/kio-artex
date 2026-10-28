"use client";

import { useEffect, useState } from "react";

import { useRouter } from "next/navigation";

import { Eye, EyeOff } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import { auth } from "@/lib/auth";

export default function SetupPage() {
  const router = useRouter();
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [checking, setChecking] = useState(true);
  // 查不到初始化状态时不能默认当成"未初始化"——那样会把初始化表单摆给一个
  // 其实已经设过密码的实例，用户照着填就会覆盖掉原密码。此时关闭入口。
  const [unavailable, setUnavailable] = useState(false);

  useEffect(() => {
    api
      .authStatus()
      .then(({ initialized }) => {
        if (initialized) router.replace("/login");
      })
      .catch((err) => {
        setError(err instanceof Error ? err.message : "无法连接到后端服务");
        setUnavailable(true);
      })
      .finally(() => setChecking(false));
  }, [router]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (password !== confirm) {
      setError("两次输入的密码不一致");
      return;
    }
    if (password.length < 8) {
      setError("密码长度至少 8 位");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const { token } = await api.initPassword(password);
      auth.setToken(token);
      router.replace("/dashboard");
    } catch (err) {
      setError(err instanceof Error ? err.message : "初始化失败");
    } finally {
      setLoading(false);
    }
  }

  if (checking) return null;

  return (
    <div className="flex min-h-dvh">
      {/* Left panel */}
      <div className="hidden flex-col items-center justify-center bg-primary p-12 text-center lg:flex lg:w-1/3">
        <div className="relative flex items-center justify-center">
          <div className="absolute size-80 rounded-full border border-primary-foreground/10" />
          <div className="absolute size-60 rounded-full border border-primary-foreground/15" />
          <div className="absolute size-40 rounded-full border border-primary-foreground/20" />
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src="/logo.png" alt="ARTEX" width={160} height={160} className="relative brightness-0 invert" />
        </div>
      </div>

      {/* Right panel */}
      <div className="flex min-h-dvh w-full items-center justify-center bg-background px-5 py-10 lg:w-2/3 lg:p-8">
        <div className="w-full max-w-md space-y-8 lg:space-y-10 lg:py-24">
          <div className="space-y-4 text-center">
            <div className="mx-auto flex size-14 items-center justify-center rounded-2xl bg-primary/10 lg:hidden">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img src="/logo.png" alt="" width={36} height={36} />
            </div>
            <h2 className="font-medium text-2xl tracking-tight">
              {unavailable ? "无法确认初始化状态" : "初始化密码"}
            </h2>
            <p className="mx-auto max-w-xl text-muted-foreground">
              {unavailable
                ? "后端或数据库暂时不可用。为避免覆盖实例上已有的密码，初始化入口已临时关闭——请恢复服务后重试。"
                : "首次使用 ARTEX，请为账户设置一个登录密码（至少 8 位）"}
            </p>
          </div>
          {unavailable ? (
            <div className="flex flex-col gap-4">
              {error && (
                <p role="alert" aria-live="assertive" className="text-center text-destructive text-sm">
                  {error}
                </p>
              )}
              <Button type="button" className="w-full" onClick={() => window.location.reload()}>
                重试
              </Button>
            </div>
          ) : (
            <form onSubmit={handleSubmit} className="flex flex-col gap-4">
              <div className="space-y-1.5">
                <Label htmlFor="password">新密码</Label>
                <div className="relative">
                  <Input
                    id="password"
                    type={showPassword ? "text" : "password"}
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder="至少 8 位"
                    autoFocus
                    autoComplete="new-password"
                    className="h-11 pr-12"
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="absolute top-1/2 right-1 -translate-y-1/2"
                    onClick={() => setShowPassword((value) => !value)}
                    aria-label={showPassword ? "隐藏密码" : "显示密码"}
                    aria-pressed={showPassword}
                  >
                    {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                  </Button>
                </div>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="confirm">确认密码</Label>
                <Input
                  id="confirm"
                  type={showPassword ? "text" : "password"}
                  value={confirm}
                  onChange={(e) => setConfirm(e.target.value)}
                  placeholder="再次输入密码"
                  autoComplete="new-password"
                  className="h-11"
                />
              </div>
              {error && (
                <p role="alert" aria-live="assertive" className="rounded-lg bg-destructive/10 px-3 py-2 text-destructive text-sm">
                  {error}
                </p>
              )}
              <Button type="submit" className="h-11 w-full" disabled={loading || !password || !confirm}>
                {loading ? "保存中..." : "设置密码并登录"}
              </Button>
              {(!password || !confirm) && (
                <p className="text-center text-muted-foreground text-xs">请设置并确认至少 8 位密码</p>
              )}
            </form>
          )}
        </div>
      </div>
    </div>
  );
}
