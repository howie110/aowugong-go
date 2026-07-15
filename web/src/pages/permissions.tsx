import { RefreshCw, UserPlus } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { notify } from "@/lib/notify";
import { PermissionUser, RoleRead, addRoleToUser, fetchPermissionRoles, fetchPermissionUsers } from "@/lib/permissions";

export function PermissionsPage() {
  const [users, setUsers] = useState<PermissionUser[]>([]);
  const [roles, setRoles] = useState<RoleRead[]>([]);
  const [selectedRoleByUser, setSelectedRoleByUser] = useState<Record<number, string>>({});
  const [isLoading, setIsLoading] = useState(true);
  const [savingUserId, setSavingUserId] = useState<number | null>(null);

  const roleNameMap = useMemo(() => Object.fromEntries(roles.map((role) => [role.code, role.name])), [roles]);

  async function loadData() {
    setIsLoading(true);
    try {
      const [nextUsers, nextRoles] = await Promise.all([fetchPermissionUsers(), fetchPermissionRoles()]);
      setUsers(nextUsers);
      setRoles(nextRoles);
      setSelectedRoleByUser(
        Object.fromEntries(
          nextUsers.map((user) => [
            user.id,
            nextRoles.find((role) => !user.roles.includes(role.code))?.code || nextRoles[0]?.code || "",
          ]),
        ),
      );
    } catch (error) {
      notify.errorFrom(error, "权限数据加载失败", "加载失败");
    } finally {
      setIsLoading(false);
    }
  }

  useEffect(() => {
    void loadData();
  }, []);

  async function handleAddRole(user: PermissionUser) {
    const roleCode = selectedRoleByUser[user.id];
    if (!roleCode) {
      notify.warning("请选择角色");
      return;
    }
    setSavingUserId(user.id);
    try {
      const updatedUser = await addRoleToUser(user.id, roleCode);
      setUsers((currentUsers) => currentUsers.map((item) => (item.id === updatedUser.id ? updatedUser : item)));
      setSelectedRoleByUser((current) => ({
        ...current,
        [user.id]: roles.find((role) => !updatedUser.roles.includes(role.code))?.code || roleCode,
      }));
      notify.success("角色已加入", `已把 ${user.username} 加入 ${roleNameMap[roleCode] || roleCode}`);
    } catch (error) {
      notify.errorFrom(error, "加入角色失败");
    } finally {
      setSavingUserId(null);
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">用户角色</h2>
          <p className="mt-1 text-sm text-muted-foreground">这里只负责把用户加入角色，角色权限由系统预设维护。</p>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={() => void loadData()} disabled={isLoading}>
          <RefreshCw className="h-4 w-4" />
          刷新
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>角色分配</CardTitle>
          <CardDescription>选择用户和角色后点击加入角色，重复加入会保持不变。</CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>用户</TableHead>
                <TableHead>当前角色</TableHead>
                <TableHead>加入角色</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {users.map((user) => (
                <TableRow key={user.id}>
                  <TableCell>
                    <div className="font-medium">{user.username}</div>
                    <div className="text-xs text-muted-foreground">{user.email}</div>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {user.roles.length ? (
                        user.roles.map((roleCode) => (
                          <Badge key={roleCode} variant={roleCode === "admin" ? "default" : "outline"}>
                            {roleNameMap[roleCode] || roleCode}
                          </Badge>
                        ))
                      ) : (
                        <Badge variant="secondary">无角色</Badge>
                      )}
                    </div>
                  </TableCell>
                  <TableCell>
                    <select
                      className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-1 focus-visible:ring-ring"
                      value={selectedRoleByUser[user.id] || ""}
                      onChange={(event) =>
                        setSelectedRoleByUser((current) => ({
                          ...current,
                          [user.id]: event.target.value,
                        }))
                      }
                    >
                      {roles.map((role) => (
                        <option key={role.code} value={role.code}>
                          {role.name}
                        </option>
                      ))}
                    </select>
                  </TableCell>
                  <TableCell className="text-right">
                    <Button type="button" size="sm" onClick={() => void handleAddRole(user)} disabled={savingUserId === user.id}>
                      <UserPlus className="h-4 w-4" />
                      加入
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          {isLoading ? <div className="py-6 text-sm text-muted-foreground">正在加载权限数据...</div> : null}
        </CardContent>
      </Card>
    </div>
  );
}
