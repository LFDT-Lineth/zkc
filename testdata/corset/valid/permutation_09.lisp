;; Permutation declared in the root module, whose target is in a module declared
;; afterwards.
(defcolumns (X :i16) (one :i1 :constant 1))
(defpermutation test ((one X)) ((m1.one m1.A)))

(module m1)
(defcolumns (A :i16) (one :i1 :constant 1))
