"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { AvatarUpload } from "@/components/avatar-upload";

interface ShopSetupFormProps {
  onSubmit: (data: { name: string; website?: string; title?: string; avatar_url?: string }) => void;
  isLoading: boolean;
}

export function ShopSetupForm({ onSubmit, isLoading }: ShopSetupFormProps) {
  const [name, setName] = useState("");
  const [website, setWebsite] = useState("");
  const [title, setTitle] = useState("");
  const [avatarUrl, setAvatarUrl] = useState("");

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;

    onSubmit({
      name: name.trim(),
      website: website.trim() || undefined,
      title: title.trim() || undefined,
      avatar_url: avatarUrl || undefined,
    });
  };

  return (
    <Card className="max-w-lg mx-auto">
      <CardHeader>
        <CardTitle>Set Up Your Shop</CardTitle>
        <CardDescription>
          Tell us about your business. You can update these details later.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-6">
          <div className="space-y-2">
            <Label htmlFor="avatar">Avatar</Label>
            <AvatarUpload
              value={avatarUrl}
              onChange={setAvatarUrl}
              fallback={name ? name[0].toUpperCase() : "S"}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="name">Shop Name *</Label>
            <Input
              id="name"
              placeholder="My Awesome Shop"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="title">Tagline</Label>
            <Input
              id="title"
              placeholder="Short description of your shop"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="website">Website</Label>
            <Input
              id="website"
              type="url"
              placeholder="https://example.com"
              value={website}
              onChange={(e) => setWebsite(e.target.value)}
            />
          </div>

          <Button type="submit" className="w-full" disabled={isLoading || !name.trim()}>
            {isLoading ? "Creating..." : "Create Shop"}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
