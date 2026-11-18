"use client";

import * as React from "react";

import { Trash2Icon, UserPlusIcon } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import type { RbacRole, RbacUser } from "@/lib/types";

const ROLE_LABEL: Record<string, string> = {
  admin: "管理员",
  operator: "操作员",
  viewer: "只读用户",
};

export default function RbacPage() {
  const [users, setUsers] = React.useState<RbacUser[]>([]);
  const [roles, setRoles] = React.useState<RbacRole[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [createOpen, setCreateOpen] = React.useState(false);
  const [editUser, setEditUser] = React.useState<RbacUser | null>(null);
  const [resetUser, setResetUser] = React.useState<RbacUser | null>(null);

  const load = React.useCallback(async () => {
    setLoading(true);
    try {
      const [u, r] = await Promise.all([api.rbacUsers(), api.rbacRoles()]);
      setUsers(u);
      setRoles(r);
    } catch (e) {
      toast.error((e as Error).message || "加载用户失败");
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    void load();
  }, [load]);

  return (
    <div className="space-y-6 p-4 lg:p-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="font-semibold text-xl">用户与角色</h1>
          <p className="mt-1 text-muted-foreground text-sm">
            固定三角色：管理员 / 操作员 / 只读用户。当前不做任务与资产的资源归属隔离。
          </p>
        </div>
        <Button onClick={() => setCreateOpen(true)}>
          <UserPlusIcon className="size-4" />
          新建用户
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>平台账号</CardTitle>
          <CardDescription>共 {users.length} 个账号。管理员可创建、停用、重置密码与删除用户。</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="border-b text-muted-foreground">
                <tr className="text-left">
                  <th className="py-2 pr-4 font-medium">用户名</th>
                  <th className="py-2 pr-4 font-medium">显示名</th>
                  <th className="py-2 pr-4 font-medium">角色</th>
                  <th className="py-2 pr-4 font-medium">状态</th>
                  <th className="py-2 pr-4 font-medium">操作</th>
                </tr>
              </thead>
              <tbody>
                {loading ? (
                  <tr>
                    <td colSpan={5} className="py-8 text-center text-muted-foreground">加载中…</td>
                  </tr>
                ) : users.length === 0 ? (
                  <tr>
                    <td colSpan={5} className="py-8 text-center text-muted-foreground">暂无用户</td>
                  </tr>
                ) : (
                  users.map((u) => (
                    <tr key={u.id} className="border-b last:border-0">
                      <td className="py-3 pr-4 font-mono">{u.username}</td>
                      <td className="py-3 pr-4">{u.display_name || "—"}</td>
                      <td className="py-3 pr-4">
                        <div className="flex flex-wrap gap-1">
                          {u.role_ids.map((id) => (
                            <Badge key={id} variant="secondary">{ROLE_LABEL[id] ?? id}</Badge>
                          ))}
                        </div>
                      </td>
                      <td className="py-3 pr-4">
                        <Badge variant={u.enabled ? "default" : "destructive"}>{u.enabled ? "启用" : "停用"}</Badge>
                      </td>
                      <td className="py-3 pr-4">
                        <div className="flex flex-wrap gap-1">
                          <Button size="sm" variant="outline" onClick={() => setEditUser(u)}>编辑</Button>
                          <Button size="sm" variant="outline" onClick={() => setResetUser(u)}>重置密码</Button>
                          {!u.is_builtin && (
                            <Button
                              size="sm"
                              variant="ghost"
                              className="text-destructive"
                              onClick={async () => {
                                if (!window.confirm(`确认删除用户 ${u.username}？`)) return;
                                try {
                                  await api.rbacDeleteUser(u.id);
                                  toast.success("已删除");
                                  await load();
                                } catch (e) {
                                  toast.error((e as Error).message || "删除失败");
                                }
                              }}
                            >
                              <Trash2Icon className="size-4" />
                            </Button>
                          )}
                        </div>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      <CreateUserDialog
        open={createOpen}
        roles={roles}
        onOpenChange={setCreateOpen}
        onCreated={async () => { setCreateOpen(false); await load(); }}
      />
      {editUser && (
        <EditUserDialog
          user={editUser}
          roles={roles}
          onClose={() => setEditUser(null)}
          onSaved={async () => { setEditUser(null); await load(); }}
        />
      )}
      {resetUser && (
        <ResetPasswordDialog
          user={resetUser}
          onClose={() => setResetUser(null)}
          onSaved={async () => { setResetUser(null); await load(); }}
        />
      )}
    </div>
  );
}

function RoleCheckboxes({ roles, value, onChange }: { roles: RbacRole[]; value: string[]; onChange: (v: string[]) => void }) {
  return (
    <div className="flex flex-wrap gap-3">
      {roles.map((r) => (
        <label key={r.id} className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={value.includes(r.id)}
            onChange={(e) => onChange(e.target.checked ? [...value, r.id] : value.filter((x) => x !== r.id))}
          />
          {ROLE_LABEL[r.id] ?? r.name}
        </label>
      ))}
    </div>
  );
}

function CreateUserDialog({ open, roles, onOpenChange, onCreated }: {
  open: boolean;
  roles: RbacRole[];
  onOpenChange: (v: boolean) => void;
  onCreated: () => void;
}) {
  const [username, setUsername] = React.useState("");
  const [displayName, setDisplayName] = React.useState("");
  const [password, setPassword] = React.useState("");
  const [roleIDs, setRoleIDs] = React.useState<string[]>(["viewer"]);
  const [busy, setBusy] = React.useState(false);

  async function submit() {
    if (!username.trim() || password.length < 8) {
      toast.error("用户名必填，密码至少 8 位");
      return;
    }
    setBusy(true);
    try {
      await api.rbacCreateUser({ username: username.trim(), display_name: displayName.trim(), password, role_ids: roleIDs });
      toast.success("用户已创建");
      setUsername(""); setDisplayName(""); setPassword(""); setRoleIDs(["viewer"]);
      onCreated();
    } catch (e) {
      toast.error((e as Error).message || "创建失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>新建用户</DialogTitle>
          <DialogDescription>创建后由管理员把账号密码交给成员；不开放自助注册。</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label>用户名</Label>
            <Input value={username} onChange={(e) => setUsername(e.target.value)} placeholder="alice" autoComplete="off" />
          </div>
          <div className="space-y-1.5">
            <Label>显示名</Label>
            <Input value={displayName} onChange={(e) => setDisplayName(e.target.value)} placeholder="张三" />
          </div>
          <div className="space-y-1.5">
            <Label>初始密码</Label>
            <Input value={password} onChange={(e) => setPassword(e.target.value)} type="password" autoComplete="new-password" />
          </div>
          <div className="space-y-1.5">
            <Label>角色</Label>
            <RoleCheckboxes roles={roles} value={roleIDs} onChange={setRoleIDs} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>取消</Button>
          <Button onClick={submit} disabled={busy}>{busy ? "创建中…" : "创建"}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function EditUserDialog({ user, roles, onClose, onSaved }: {
  user: RbacUser;
  roles: RbacRole[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [displayName, setDisplayName] = React.useState(user.display_name);
  const [enabled, setEnabled] = React.useState(user.enabled);
  const [roleIDs, setRoleIDs] = React.useState<string[]>(user.role_ids);
  const [busy, setBusy] = React.useState(false);

  async function submit() {
    setBusy(true);
    try {
      await api.rbacUpdateUser(user.id, { display_name: displayName, enabled, role_ids: roleIDs });
      toast.success("已保存");
      onSaved();
    } catch (e) {
      toast.error((e as Error).message || "保存失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>编辑 {user.username}</DialogTitle>
          <DialogDescription>停用账号会立即作废该用户已签发的 token。</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label>显示名</Label>
            <Input value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
          </div>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
            启用账号
          </label>
          <div className="space-y-1.5">
            <Label>角色</Label>
            <RoleCheckboxes roles={roles} value={roleIDs} onChange={setRoleIDs} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>取消</Button>
          <Button onClick={submit} disabled={busy}>{busy ? "保存中…" : "保存"}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ResetPasswordDialog({ user, onClose, onSaved }: { user: RbacUser; onClose: () => void; onSaved: () => void }) {
  const [password, setPassword] = React.useState("");
  const [busy, setBusy] = React.useState(false);

  async function submit() {
    if (password.length < 8) {
      toast.error("密码至少 8 位");
      return;
    }
    setBusy(true);
    try {
      await api.rbacUpdateUser(user.id, { password });
      toast.success("密码已重置");
      onSaved();
    } catch (e) {
      toast.error((e as Error).message || "重置失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>重置 {user.username} 的密码</DialogTitle>
          <DialogDescription>重置后该用户所有旧 token 立即失效。</DialogDescription>
        </DialogHeader>
        <Input value={password} onChange={(e) => setPassword(e.target.value)} type="password" autoComplete="new-password" placeholder="新密码（至少 8 位）" />
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>取消</Button>
          <Button onClick={submit} disabled={busy}>{busy ? "提交中…" : "重置"}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
