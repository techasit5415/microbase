import {page} from 'vitest/browser';
import {expect,test,vi} from 'vitest';
import {render} from 'vitest-browser-svelte';
import PaginationBar from './PaginationBar.svelte';
test('first page disables previous and next emits page 2',async()=>{
 const onPageChange=vi.fn();render(PaginationBar,{page:1,perPage:20,totalItems:21,onPageChange});
 await expect.element(page.getByRole('button',{name:'Prev'})).toBeDisabled();
 await page.getByRole('button',{name:'Next'}).click();expect(onPageChange).toHaveBeenCalledWith(2);
});
test('empty collection disables both navigation buttons',async()=>{
 render(PaginationBar,{page:1,perPage:20,totalItems:0,onPageChange:vi.fn()});
 await expect.element(page.getByRole('button',{name:'Next'})).toBeDisabled();
 await expect.element(page.getByRole('button',{name:'Prev'})).toBeDisabled();
});
