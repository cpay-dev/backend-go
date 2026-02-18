"use client";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Store, User } from "lucide-react";

interface RoleSelectorProps {
  onSelect: (role: "merchant" | "user") => void;
  isLoading: boolean;
}

export function RoleSelector({ onSelect, isLoading }: RoleSelectorProps) {
  return (
    <div className="grid grid-cols-1 gap-6 md:grid-cols-2 max-w-2xl mx-auto">
      <Card
        className={`cursor-pointer transition-all hover:border-primary hover:shadow-lg ${isLoading ? "pointer-events-none opacity-50" : ""}`}
        onClick={() => onSelect("merchant")}
      >
        <CardHeader className="text-center">
          <div className="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-primary/10">
            <Store className="h-8 w-8 text-primary" />
          </div>
          <CardTitle>Merchant</CardTitle>
          <CardDescription>
            Create a shop, publish products, accept crypto payments
          </CardDescription>
        </CardHeader>
        <CardContent className="text-center text-sm text-muted-foreground">
          Set up payment links, invoices, and recurring subscriptions
        </CardContent>
      </Card>

      <Card
        className={`cursor-pointer transition-all hover:border-primary hover:shadow-lg ${isLoading ? "pointer-events-none opacity-50" : ""}`}
        onClick={() => onSelect("user")}
      >
        <CardHeader className="text-center">
          <div className="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-primary/10">
            <User className="h-8 w-8 text-primary" />
          </div>
          <CardTitle>User</CardTitle>
          <CardDescription>
            View transaction history and manage payments
          </CardDescription>
        </CardHeader>
        <CardContent className="text-center text-sm text-muted-foreground">
          Track your payments, subscriptions, and receipts
        </CardContent>
      </Card>
    </div>
  );
}
