declare module "which" {
  interface Options {
    nothrow?: boolean;
    path?: string;
    pathExt?: string;
    all?: boolean;
  }

  interface WhichStatic {
    sync(cmd: string, options: Options & { nothrow: true }): string | null;
    sync(cmd: string, options?: Options): string;
    (cmd: string, options?: Options): Promise<string>;
  }

  const which: WhichStatic;
  export default which;
}
