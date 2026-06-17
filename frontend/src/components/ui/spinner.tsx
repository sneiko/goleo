import { Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";

export function Spinner({ className }: { className?: string }) {
  return <Loader2 aria-hidden="true" className={cn("animate-spin", className)} data-icon="inline-start" />;
}
