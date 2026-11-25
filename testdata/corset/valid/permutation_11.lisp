;; Permutation declared in the root module, before the module which holds its
;; columns.
(defpermutation test ((m1.one m1.X)) ((m1.one m1.Y)))
(module m1)
(defcolumns (X :i16) (Y :i16) (one :i1 :constant 1))
