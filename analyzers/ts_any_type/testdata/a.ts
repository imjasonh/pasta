function f(x: any): any {           // want "`any` defeats"
    return x;                        // want:-1 "`any` defeats"
}

function g(y: number): string {
    return String(y);
}

let opaque: unknown = null;          // OK
let typed: number = 5;               // OK
const arr: any[] = [];               // want "`any` defeats"
