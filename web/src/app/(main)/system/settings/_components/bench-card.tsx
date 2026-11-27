"use client";

import { FlaskConicalIcon } from "lucide-react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

const COMMANDS: { label: string; cmd: string; hint: string }[] = [
  {
    label: "校验场景库",
    cmd: "kaiyang-maint bench validate",
    hint: "检查内置 / 自定义场景 manifest 是否合法。",
  },
  {
    label: "给一次运行打分",
    cmd: "kaiyang-maint bench score --config config.json --scenario dvwa-sqli-xss --task 18 --out run-18.json",
    hint: "只读已落库的任务结果，不重跑 agent、不发网络请求。",
  },
  {
    label: "对比两次运行",
    cmd: "kaiyang-maint bench compare --a run-18.json --b run-42.json --markdown diff.md",
    hint: "PASS→FAIL 退步时退出码为 1，可直接接 CI。",
  },
];

/** 离线评测入口：只展示命令说明，不在页面内在线运行。 */
export function BenchCard({ className }: { className?: string }) {
  return (
    <Card className={className}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <FlaskConicalIcon className="size-4" />
          离线评测（bench）
        </CardTitle>
        <CardDescription>
          评测 harness 在服务器命令行运行：离线、只读，不启动靶场、不重跑 agent。 靶场按场景 manifest 的{" "}
          <code>setup</code> 启动，任务照常在 Web/API 里创建并跑完后，再用 task id 打分。
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {COMMANDS.map((c) => (
          <div key={c.cmd} className="flex flex-col gap-1">
            <span className="font-medium text-sm">{c.label}</span>
            <code className="break-all rounded-md border bg-muted/40 px-2.5 py-1.5 font-mono text-xs">{c.cmd}</code>
            <span className="text-muted-foreground text-xs">{c.hint}</span>
          </div>
        ))}
        <p className="text-muted-foreground text-xs">
          详细指标与场景 manifest 见仓库 <code>eval/README.md</code>。页面不提供在线运行入口。
        </p>
      </CardContent>
    </Card>
  );
}
