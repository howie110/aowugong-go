import { Plus, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldLabel } from "@/components/ui/field";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { type VPNUserOption, type VPNSummary } from "@/lib/vpn";

export function CreateUserDialog({
  open,
  userID,
  profileCode,
  users,
  profiles,
  isSaving,
  onOpenChange,
  onUserChange,
  onProfileChange,
  onSave,
}: {
  open: boolean;
  userID: string;
  profileCode: string;
  users: VPNUserOption[];
  profiles: VPNSummary["profiles"];
  isSaving: boolean;
  onOpenChange: (open: boolean) => void;
  onUserChange: (userID: string) => void;
  onProfileChange: (code: string) => void;
  onSave: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>开通用户订阅</DialogTitle>
          <DialogDescription>同一登录用户可以分配多套 VPN 资源，每套资源可在多台终端中共用。</DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <Field>
            <FieldLabel>登录用户</FieldLabel>
            <Select value={userID} onValueChange={onUserChange}>
              <SelectTrigger>
                <SelectValue placeholder="选择用户" />
              </SelectTrigger>
              <SelectContent>
                {users.map((user) => (
                  <SelectItem key={user.id} value={String(user.id)}>{user.username} · {user.email}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel>VPN 资源</FieldLabel>
            <Select value={profileCode} onValueChange={onProfileChange}>
              <SelectTrigger>
                <SelectValue placeholder="选择资源" />
              </SelectTrigger>
              <SelectContent>
                {profiles.map((profile) => <SelectItem key={profile.code} value={profile.code}>{profile.name}</SelectItem>)}
              </SelectContent>
            </Select>
          </Field>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={isSaving}>取消</Button>
          <Button type="button" onClick={onSave} disabled={isSaving}>
            {isSaving ? <RefreshCw className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
            确认开通
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
