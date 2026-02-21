"use client";

import { useCallback, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Upload, X } from "lucide-react";
import { api } from "@/lib/api";

interface ImageUploadProps {
  value?: string;
  onChange: (url: string) => void;
  label?: string;
}

export function ImageUpload({ value, onChange, label = "Upload Image" }: ImageUploadProps) {
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
        const { url } = await api.upload.productImage(file);
        onChange(url);
      } catch (err) {
        console.error("Upload failed:", err);
        alert("Failed to upload image");
      } finally {
        setIsUploading(false);
        if (fileRef.current) {
          fileRef.current.value = "";
        }
      }
    },
    [onChange]
  );

  return (
    <div className="space-y-2">
      {value && (
        <div className="relative inline-block">
          <img
            src={value}
            alt="Product"
            className="h-32 w-32 rounded-lg border object-cover"
          />
          <button
            type="button"
            onClick={() => onChange("")}
            className="absolute -right-2 -top-2 rounded-full bg-destructive p-1 text-destructive-foreground"
          >
            <X className="h-3 w-3" />
          </button>
        </div>
      )}
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
          {isUploading ? "Uploading..." : label}
        </Button>
        <p className="mt-1 text-xs text-muted-foreground">PNG, JPG, WebP. Max 5MB.</p>
      </div>
    </div>
  );
}
