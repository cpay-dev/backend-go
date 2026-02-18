"use client";

import { useCallback, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Upload } from "lucide-react";
import { api } from "@/lib/api";

interface AvatarUploadProps {
  value?: string;
  onChange: (url: string) => void;
  fallback?: string;
}

export function AvatarUpload({ value, onChange, fallback = "?" }: AvatarUploadProps) {
  const fileRef = useRef<HTMLInputElement>(null);
  const [isUploading, setIsUploading] = useState(false);

  const handleFileChange = useCallback(
    async (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      if (!file) return;

      if (file.size > 5 * 1024 * 1024) {
        alert("File too large (max 5MB)");
        return;
      }

      if (!["image/png", "image/jpeg", "image/webp"].includes(file.type)) {
        alert("Invalid file type (allowed: PNG, JPEG, WebP)");
        return;
      }

      setIsUploading(true);
      try {
        const { url } = await api.upload.avatar(file);
        onChange(url);
      } catch (err) {
        console.error("Upload failed:", err);
        alert("Failed to upload avatar");
      } finally {
        setIsUploading(false);
      }
    },
    [onChange]
  );

  return (
    <div className="flex items-center gap-4">
      <Avatar className="h-16 w-16">
        <AvatarImage src={value} />
        <AvatarFallback>{fallback}</AvatarFallback>
      </Avatar>
      <div>
        <input
          ref={fileRef}
          type="file"
          accept="image/png,image/jpeg,image/webp"
          className="hidden"
          onChange={handleFileChange}
        />
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={isUploading}
          onClick={() => fileRef.current?.click()}
        >
          <Upload className="mr-2 h-4 w-4" />
          {isUploading ? "Uploading..." : "Upload Avatar"}
        </Button>
        <p className="mt-1 text-xs text-muted-foreground">PNG, JPG, WebP. Max 5MB.</p>
      </div>
    </div>
  );
}
