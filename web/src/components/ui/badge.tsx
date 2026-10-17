import * as React from "react";
import {cva,type VariantProps} from "class-variance-authority";
import {cn} from "../../lib/utils";

const badgeVariants=cva("enterprise-badge inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-semibold",{
 variants:{variant:{
  default:"enterprise-badge-default",
  success:"enterprise-badge-success",
  warning:"enterprise-badge-warning",
  neutral:"enterprise-badge-neutral"
 }},
 defaultVariants:{variant:"default"}
});
export interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement>,VariantProps<typeof badgeVariants>{}
export function Badge({className,variant,...props}:BadgeProps){
 return <span className={cn(badgeVariants({variant}),className)} {...props}/>;
}
