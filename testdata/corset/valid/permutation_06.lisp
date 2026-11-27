;; Permutation declared in the module of its target, whose source is in another
;; module.
(module m1)
(defcolumns (X :i16) (one :i1 :constant 1))

(module m2)
(defcolumns (Y :i16) (one :i1 :constant 1))
(defpermutation test ((m1.one m1.X)) ((one Y)))
