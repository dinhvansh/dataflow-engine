import * as React from "react";
import {Slot} from "@radix-ui/react-slot";
import {cva,type VariantProps} from "class-variance-authority";
import {cn} from "../../lib/utils";

// DataFlow's shadcn-style Button uses semantic tokens instead of Tailwind
// palette literals. A single component remains compatible with Light/Dark.
const variants=cva(
 "df-button inline-flex items-center justify-center gap-2 whitespace-nowrap text-sm font-medium transition-colors disabled:pointer-events-none disabled:opacity-50",
 {
  variants:{
   variant:{
    default:"df-btn-primary",
    outline:"df-btn-outline",
    ghost:"df-btn-ghost",
    destructive:"df-btn-danger",
   },
   size:{
    default:"h-9 px-4 py-2",
    sm:"h-8 px-3",
    lg:"h-11 px-6",
    icon:"h-9 w-9",
   },
  },
  defaultVariants:{variant:"default",size:"default"},
 }
);
export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement>,VariantProps<typeof variants>{asChild?:boolean}
const Button=React.forwardRef<HTMLButtonElement,ButtonProps>(({className,variant,size,asChild=false,...props},ref)=>{
 const Comp=asChild?Slot:"button";
 return <Comp className={cn(variants({variant,size,className}))} ref={ref} {...props}/>;
});
Button.displayName="Button";
export {Button};
