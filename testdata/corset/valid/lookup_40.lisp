;; As lookup_09, but without a source selector.
(defcolumns (X :i16))
(deflookup test ((_ X)) ((m1.one m1.A)))

(module m1)
(defcolumns (A :i16) (one :i1 :constant 1))
